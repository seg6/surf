#import "RBH264SampleBuilder.h"

#include "surf/h264.h"

@interface RBH264SampleBuilder () {
    CMVideoFormatDescriptionRef _formatDescription;
}
@property(nonatomic, strong) NSData *currentSPS;
@property(nonatomic, strong) NSData *currentPPS;
@end

@implementation RBH264SampleBuilder

- (id)initWithWidth:(int)width height:(int)height {
    self = [super init];
    if (self) {
        self.codedWidth = width;
        self.codedHeight = height;
    }
    return self;
}

- (CMVideoFormatDescriptionRef)formatDescription {
    return _formatDescription;
}

- (void)reset {
    if (_formatDescription) {
        CFRelease(_formatDescription);
        _formatDescription = NULL;
    }
    self.currentSPS = nil;
    self.currentPPS = nil;
}

- (BOOL)updateFormatFromInfo:(const surf_h264_au_info_t *)info
                     changed:(BOOL *)changed
                      status:(OSStatus *)statusOut {
    if (changed) *changed = NO;
    if (!info->sps || !info->pps) return _formatDescription != NULL;

    NSData *sps = [NSData dataWithBytes:info->sps length:info->sps_length];
    NSData *pps = [NSData dataWithBytes:info->pps length:info->pps_length];
    if (_formatDescription && [sps isEqualToData:self.currentSPS] &&
        [pps isEqualToData:self.currentPPS]) return YES;

    size_t avccCapacity = 11 + info->sps_length + info->pps_length;
    uint8_t *avcc = malloc(avccCapacity);
    if (!avcc) {
        if (statusOut) *statusOut = -108; // classic Mac/iOS memFullErr
        return NO;
    }
    size_t avccLength = surf_h264_build_avcc_config(
        info->sps, info->sps_length, info->pps, info->pps_length,
        avcc, avccCapacity);
    if (!avccLength) {
        free(avcc);
        if (statusOut) *statusOut = kCMFormatDescriptionError_InvalidParameter;
        return NO;
    }

    CFDataRef avccData = CFDataCreate(NULL, avcc, (CFIndex)avccLength);
    free(avcc);
    CFStringRef avcCKey = CFSTR("avcC");
    CFDictionaryRef atoms = CFDictionaryCreate(NULL,
        (const void **)&avcCKey, (const void **)&avccData, 1,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFStringRef extensionKey = kCMFormatDescriptionExtension_SampleDescriptionExtensionAtoms;
    CFDictionaryRef extensions = CFDictionaryCreate(NULL,
        (const void **)&extensionKey, (const void **)&atoms, 1,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);

    CMVideoFormatDescriptionRef nextFormat = NULL;
    OSStatus status = CMVideoFormatDescriptionCreate(NULL, kCMVideoCodecType_H264,
        MAX(2, self.codedWidth), MAX(2, self.codedHeight), extensions, &nextFormat);
    CFRelease(extensions);
    CFRelease(atoms);
    CFRelease(avccData);
    if (statusOut) *statusOut = status;
    if (status != noErr || !nextFormat) return NO;

    if (_formatDescription) CFRelease(_formatDescription);
    _formatDescription = nextFormat;
    self.currentSPS = sps;
    self.currentPPS = pps;
    if (changed) *changed = YES;
    return YES;
}

- (CMSampleBufferRef)createSampleForAU:(NSData *)au
                                   idr:(BOOL)idr
                         formatChanged:(BOOL *)formatChanged
                                status:(OSStatus *)statusOut {
    if (statusOut) *statusOut = noErr;
    if (formatChanged) *formatChanged = NO;
    if (![au length]) {
        if (statusOut) *statusOut = kCMFormatDescriptionError_InvalidParameter;
        return NULL;
    }

    const uint8_t *bytes = (const uint8_t *)[au bytes];
    size_t length = [au length];
    surf_h264_au_info_t info;
    if (surf_h264_scan_annexb(bytes, length, &info) != 0 || !info.has_slice) {
        if (statusOut) *statusOut = kCMFormatDescriptionError_InvalidParameter;
        return NULL;
    }
    if (![self updateFormatFromInfo:&info changed:formatChanged status:statusOut]) return NULL;

    size_t capacity = info.avcc_length;
    if (!capacity) {
        if (statusOut) *statusOut = kCMFormatDescriptionError_InvalidParameter;
        return NULL;
    }
    uint8_t *avccBytes = malloc(capacity);
    if (!avccBytes) {
        if (statusOut) *statusOut = -108; // classic Mac/iOS memFullErr
        return NULL;
    }
    size_t avccLength = surf_h264_annexb_to_avcc(
        bytes, length, avccBytes, capacity);
    if (!avccLength) {
        free(avccBytes);
        if (statusOut) *statusOut = kCMFormatDescriptionError_InvalidParameter;
        return NULL;
    }

    CMBlockBufferRef block = NULL;
    OSStatus status = CMBlockBufferCreateWithMemoryBlock(NULL, avccBytes,
        avccLength, kCFAllocatorMalloc, NULL, 0, avccLength, 0, &block);
    if (status != noErr || !block) {
        free(avccBytes);
        if (statusOut) *statusOut = status;
        return NULL;
    }

    CMSampleBufferRef sample = NULL;
    size_t sampleSize = avccLength;
    status = CMSampleBufferCreate(NULL, block, true, NULL, NULL,
        _formatDescription, 1, 0, NULL, 1, &sampleSize, &sample);
    CFRelease(block);
    if (status != noErr || !sample) {
        if (statusOut) *statusOut = status;
        return NULL;
    }

    CFArrayRef attachments = CMSampleBufferGetSampleAttachmentsArray(sample, true);
    if (attachments && CFArrayGetCount(attachments) > 0) {
        CFMutableDictionaryRef attachment =
            (CFMutableDictionaryRef)CFArrayGetValueAtIndex(attachments, 0);
        CFDictionarySetValue(attachment, kCMSampleAttachmentKey_DisplayImmediately,
                             kCFBooleanTrue);
        if (!idr) {
            CFDictionarySetValue(attachment, kCMSampleAttachmentKey_NotSync,
                                 kCFBooleanTrue);
        }
    }
    if (statusOut) *statusOut = noErr;
    return sample;
}

- (void)dealloc {
    [self reset];
}

@end

/* Minimal VideoToolbox declarations for iOS 6, where the framework exists
 * but is private (public API arrived in iOS 8). We never link against it —
 * every entry point is resolved with dlopen/dlsym at runtime and the app
 * falls back to the JPEG lane if resolution fails.
 *
 * Sources for these declarations:
 *  - public iOS 8 SDK VTDecompressionSession.h / VTErrors.h (ABI-compatible
 *    for these entry points; verified against jailbreak-era iOS 6 dumps)
 *  - types below are limited to exactly what RBVideoDecoder calls.
 */
#ifndef RB_VT_PRIVATE_H
#define RB_VT_PRIVATE_H

#include <CoreFoundation/CoreFoundation.h>
#include <CoreMedia/CoreMedia.h>
#include <CoreVideo/CoreVideo.h>

typedef struct OpaqueVTDecompressionSession *VTDecompressionSessionRef;
typedef UInt32 VTDecodeFrameFlags;
typedef UInt32 VTDecodeInfoFlags;

/* VTErrors.h */
enum {
    kVTInvalidSessionErr = -12903,
};

/* Output callback: fires once per decoded frame (VT's own thread). */
typedef void (*VTDecompressionOutputCallback)(
    void *decompressionOutputRefCon,
    void *sourceFrameRefCon,
    OSStatus status,
    VTDecodeInfoFlags infoFlags,
    CVImageBufferRef imageBuffer,
    CMTime presentationTimeStamp,
    CMTime presentationDuration);

/* Exactly two fields — no version word. Verified the hard way: an extra
 * leading field shifts the callback into the refcon slot and VideoToolbox
 * jumps to NULL on the first decoded frame (device crash 2026-07-16). */
typedef struct {
    VTDecompressionOutputCallback decompressionOutputCallback;
    void *decompressionOutputRefCon;
} VTDecompressionOutputCallbackRecord;

/* dlsym'd function types (names match the exported symbols). */
typedef OSStatus (*RBVTDecompressionSessionCreateFn)(
    CFAllocatorRef allocator,
    CMVideoFormatDescriptionRef videoFormatDescription,
    CFDictionaryRef videoDecoderSpecification,
    CFDictionaryRef destinationImageBufferAttributes,
    const VTDecompressionOutputCallbackRecord *outputCallback,
    VTDecompressionSessionRef *decompressionSessionOut);

typedef OSStatus (*RBVTDecompressionSessionDecodeFrameFn)(
    VTDecompressionSessionRef session,
    CMSampleBufferRef sampleBuffer,
    VTDecodeFrameFlags decodeFlags,
    void *sourceFrameRefCon,
    VTDecodeInfoFlags *infoFlagsOut);

typedef void (*RBVTDecompressionSessionInvalidateFn)(VTDecompressionSessionRef session);

#endif

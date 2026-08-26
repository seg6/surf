#import "RBQRScannerController.h"
#import "RBLog.h"
#import "RBTheme.h"
#import "quirc/quirc.h"
#import <AVFoundation/AVFoundation.h>
#import <QuartzCore/QuartzCore.h>
#include <string.h>

static void RBSetBrightScreenExposureBias(AVCaptureDevice *camera) {
    SEL selector = NSSelectorFromString(@"setExposureTargetBias:completionHandler:");
    NSMethodSignature *signature = [camera methodSignatureForSelector:selector];
    if (!signature) return;
    NSInvocation *invocation = [NSInvocation invocationWithMethodSignature:signature];
    float bias = -1.0f;
    id completion = nil;
    invocation.target = camera;
    invocation.selector = selector;
    [invocation setArgument:&bias atIndex:2];
    [invocation setArgument:&completion atIndex:3];
    [invocation invoke];
}

@interface RBQRScannerController () <AVCaptureMetadataOutputObjectsDelegate, AVCaptureVideoDataOutputSampleBufferDelegate> {
    dispatch_queue_t _decodeQueue;
    struct quirc *_decoder;
    CFTimeInterval _lastSoftwareDecodeAt;
}
@property(nonatomic, strong) AVCaptureSession *captureSession;
@property(nonatomic, strong) AVCaptureVideoPreviewLayer *previewLayer;
@property(nonatomic, strong) UILabel *guideLabel;
@property(nonatomic, strong) UIView *scanFrame;
@property(nonatomic, strong) UILabel *unavailableLabel;
@property(nonatomic, strong) UIButton *manualButton;
@property(nonatomic, strong) UIActivityIndicatorView *spinner;
@property(nonatomic, assign) BOOL delivered;
@end

@implementation RBQRScannerController

- (void)viewDidLoad {
    [super viewDidLoad];
    self.title = @"Scan Pairing Code";
    self.view.backgroundColor = [UIColor blackColor];
    self.navigationItem.leftBarButtonItem = [[UIBarButtonItem alloc] initWithBarButtonSystemItem:UIBarButtonSystemItemCancel
                                                                                         target:self action:@selector(cancel:)];
    self.spinner = [[UIActivityIndicatorView alloc] initWithActivityIndicatorStyle:UIActivityIndicatorViewStyleWhiteLarge];
    [self.spinner startAnimating];
    [self.view addSubview:self.spinner];
    [self prepareCameraAuthorization];
}

- (void)viewWillDisappear:(BOOL)animated {
    [super viewWillDisappear:animated];
    [self.captureSession stopRunning];
}

- (void)prepareCameraAuthorization {
    BOOL iOS7OrLater = [[[UIDevice currentDevice] systemVersion] compare:@"7.0" options:NSNumericSearch] != NSOrderedAscending;
    if (!iOS7OrLater) { [self configureCamera]; return; }
    AVAuthorizationStatus status = [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeVideo];
    if (status == AVAuthorizationStatusDenied || status == AVAuthorizationStatusRestricted) {
        [self showUnavailable:@"Camera unavailable."];
        return;
    }
    if (status == AVAuthorizationStatusNotDetermined && [AVCaptureDevice respondsToSelector:@selector(requestAccessForMediaType:completionHandler:)]) {
        [AVCaptureDevice requestAccessForMediaType:AVMediaTypeVideo completionHandler:^(BOOL granted) {
            dispatch_async(dispatch_get_main_queue(), ^{
                if (granted) [self configureCamera];
                else [self showUnavailable:@"Camera access denied."];
            });
        }];
        return;
    }
    [self configureCamera];
}

- (void)configureCamera {
    if (self.captureSession) return;
    AVCaptureDevice *camera = [AVCaptureDevice defaultDeviceWithMediaType:AVMediaTypeVideo];
    NSError *error = nil;
    if ([camera lockForConfiguration:&error]) {
        if ([camera isFocusModeSupported:AVCaptureFocusModeContinuousAutoFocus])
            camera.focusMode = AVCaptureFocusModeContinuousAutoFocus;
        if ([camera isExposureModeSupported:AVCaptureExposureModeContinuousAutoExposure])
            camera.exposureMode = AVCaptureExposureModeContinuousAutoExposure;
        if ([camera isFocusPointOfInterestSupported]) camera.focusPointOfInterest = CGPointMake(0.5, 0.5);
        // Do not meter a single point: it may land on a black QR module and
        // make the surrounding monitor white bloom. Whole-frame continuous
        // exposure is substantially steadier on the iOS 6 camera stack.
        RBSetBrightScreenExposureBias(camera);
        [camera unlockForConfiguration];
    } else {
        RBLogEvent(@"qr", @"warn", @{@"error": [error localizedDescription] ?: @""}, @"Camera configuration unavailable");
        error = nil;
    }
    AVCaptureDeviceInput *input = camera ? [AVCaptureDeviceInput deviceInputWithDevice:camera error:&error] : nil;
    AVCaptureSession *session = [[AVCaptureSession alloc] init];
    if ([session canSetSessionPreset:AVCaptureSessionPreset1920x1080])
        session.sessionPreset = AVCaptureSessionPreset1920x1080;
    else if ([session canSetSessionPreset:AVCaptureSessionPreset1280x720])
        session.sessionPreset = AVCaptureSessionPreset1280x720;
    else if ([session canSetSessionPreset:AVCaptureSessionPreset640x480])
        session.sessionPreset = AVCaptureSessionPreset640x480;
    if (!input || ![session canAddInput:input]) {
        RBLogEvent(@"qr", @"error", @{@"error": [error localizedDescription] ?: @""}, @"Camera input unavailable");
        [self showUnavailable:[error localizedDescription] ?: @"Camera unavailable."];
        return;
    }
    [session addInput:input];

    BOOL usingNativeQR = NO;
    Class metadataClass = NSClassFromString(@"AVCaptureMetadataOutput");
    if (metadataClass) {
        AVCaptureMetadataOutput *output = [[metadataClass alloc] init];
        if ([session canAddOutput:output]) {
            [session addOutput:output];
            NSString *qrType = @"org.iso.QRCode";
            if ([output.availableMetadataObjectTypes containsObject:qrType]) {
                [output setMetadataObjectsDelegate:self queue:dispatch_get_main_queue()];
                output.metadataObjectTypes = @[qrType];
                usingNativeQR = YES;
            } else {
                RBLogEvent(@"qr", @"warn", @{@"fallback": @"software"}, @"Native metadata output has no QR type");
                [session removeOutput:output];
            }
        }
    }
    if (!usingNativeQR) {
        AVCaptureVideoDataOutput *output = [[AVCaptureVideoDataOutput alloc] init];
        output.alwaysDiscardsLateVideoFrames = YES;
        output.videoSettings = @{(id)kCVPixelBufferPixelFormatTypeKey:
                                     [NSNumber numberWithUnsignedInt:kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange]};
        if (![session canAddOutput:output]) {
            RBLogEvent(@"qr", @"error", @{}, @"Video data output unavailable");
            [self showUnavailable:@"QR scanner unavailable."];
            return;
        }
        _decoder = quirc_new();
        if (!_decoder) {
            RBLogEvent(@"qr", @"error", @{}, @"Software decoder allocation failed");
            [self showUnavailable:@"QR scanner unavailable."];
            return;
        }
        _decodeQueue = dispatch_queue_create("space.seg6.surf.qr", DISPATCH_QUEUE_SERIAL);
        [output setSampleBufferDelegate:self queue:_decodeQueue];
        [session addOutput:output];
        RBLogEvent(@"qr", @"info", @{@"decoder": @"software"}, @"QR decoder active");
    }
    self.captureSession = session;
    self.previewLayer = [AVCaptureVideoPreviewLayer layerWithSession:session];
    self.previewLayer.videoGravity = AVLayerVideoGravityResizeAspectFill;
    [self.view.layer insertSublayer:self.previewLayer atIndex:0];

    self.scanFrame = [[UIView alloc] initWithFrame:CGRectZero];
    self.scanFrame.backgroundColor = [UIColor clearColor];
    self.scanFrame.layer.borderColor = [RBTheme seaGlassColor].CGColor;
    self.scanFrame.layer.borderWidth = 3.0;
    self.scanFrame.layer.cornerRadius = 14.0;
    [self.view addSubview:self.scanFrame];

    self.guideLabel = [[UILabel alloc] initWithFrame:CGRectZero];
    self.guideLabel.backgroundColor = [[RBTheme deepTideColor] colorWithAlphaComponent:0.90];
    self.guideLabel.textColor = [UIColor whiteColor];
    self.guideLabel.textAlignment = NSTextAlignmentCenter;
    self.guideLabel.numberOfLines = 2;
    self.guideLabel.font = [RBTheme displayFontOfSize:15.0];
    self.guideLabel.layer.cornerRadius = 10.0;
    self.guideLabel.layer.masksToBounds = YES;
    self.guideLabel.text = @"Point at the pairing code";
    [self.view addSubview:self.guideLabel];
    [self.spinner stopAnimating];
    self.spinner.hidden = YES;
    [session startRunning];
    [self.view setNeedsLayout];
}

- (void)viewDidLayoutSubviews {
    [super viewDidLayoutSubviews];
    self.previewLayer.frame = self.view.bounds;
    self.spinner.center = CGPointMake(CGRectGetMidX(self.view.bounds), CGRectGetMidY(self.view.bounds));
    CGFloat side = MIN(280.0, MIN(self.view.bounds.size.width, self.view.bounds.size.height) - 70.0);
    self.scanFrame.frame = CGRectMake(floorf((self.view.bounds.size.width - side) / 2.0),
                                           floorf((self.view.bounds.size.height - side) / 2.0) - 12.0, side, side);
    self.guideLabel.frame = CGRectMake(20.0, CGRectGetMaxY(self.scanFrame.frame) + 18.0,
                                      self.view.bounds.size.width - 40.0, 54.0);
    self.unavailableLabel.frame = CGRectInset(self.view.bounds, 28.0, 120.0);
    self.manualButton.frame = CGRectMake(35.0, self.view.bounds.size.height - 80.0,
                                         self.view.bounds.size.width - 70.0, 44.0);
}

- (void)showUnavailable:(NSString *)message {
    [self.captureSession stopRunning];
    self.captureSession = nil;
    [self.previewLayer removeFromSuperlayer];
    self.previewLayer = nil;
    [self.spinner stopAnimating];
    self.spinner.hidden = YES;
    self.scanFrame.hidden = YES;
    self.guideLabel.hidden = YES;
    if (!self.unavailableLabel) {
        self.unavailableLabel = [[UILabel alloc] initWithFrame:CGRectZero];
        self.unavailableLabel.backgroundColor = [UIColor clearColor];
        self.unavailableLabel.textColor = [UIColor whiteColor];
        self.unavailableLabel.textAlignment = NSTextAlignmentCenter;
        self.unavailableLabel.numberOfLines = 0;
        self.unavailableLabel.font = [RBTheme fontOfSize:17.0 bold:NO];
        [self.view addSubview:self.unavailableLabel];
        self.manualButton = [UIButton buttonWithType:UIButtonTypeCustom];
        [self.manualButton setTitle:@"Enter Address" forState:UIControlStateNormal];
        [RBTheme stylePrimaryButton:self.manualButton];
        [self.manualButton addTarget:self action:@selector(cancel:) forControlEvents:UIControlEventTouchUpInside];
        [self.view addSubview:self.manualButton];
    }
    self.unavailableLabel.text = message;
    [self.view setNeedsLayout];
}

- (void)cancel:(id)sender {
    [self.captureSession stopRunning];
    [self.delegate qrScannerDidCancel:self];
}

- (void)captureOutput:(AVCaptureOutput *)captureOutput didOutputMetadataObjects:(NSArray *)metadataObjects fromConnection:(AVCaptureConnection *)connection {
    if (self.delivered) return;
    Class codeClass = NSClassFromString(@"AVMetadataMachineReadableCodeObject");
    for (AVMetadataObject *object in metadataObjects) {
        if (!codeClass || ![object isKindOfClass:codeClass]) continue;
        NSString *value = [object performSelector:@selector(stringValue)];
        if (![value hasPrefix:@"surf://pair?"]) continue;
        self.delivered = YES;
        [self.captureSession stopRunning];
        [self.delegate qrScanner:self didScanValue:value];
        return;
    }
}

- (void)captureOutput:(AVCaptureOutput *)captureOutput didOutputSampleBuffer:(CMSampleBufferRef)sampleBuffer fromConnection:(AVCaptureConnection *)connection {
    if (self.delivered || !_decoder) return;
    CFTimeInterval now = CACurrentMediaTime();
    if (now - _lastSoftwareDecodeAt < 0.18) return;
    _lastSoftwareDecodeAt = now;
    CVPixelBufferRef pixelBuffer = CMSampleBufferGetImageBuffer(sampleBuffer);
    if (!pixelBuffer || CVPixelBufferGetPlaneCount(pixelBuffer) < 1) return;
    CVPixelBufferLockBaseAddress(pixelBuffer, kCVPixelBufferLock_ReadOnly);
    int width = (int)CVPixelBufferGetWidthOfPlane(pixelBuffer, 0);
    int height = (int)CVPixelBufferGetHeightOfPlane(pixelBuffer, 0);
    int cropSize = MIN(width, height);
    int cropX = (width - cropSize) / 2;
    int cropY = (height - cropSize) / 2;
    uint8_t *source = CVPixelBufferGetBaseAddressOfPlane(pixelBuffer, 0);
    size_t stride = CVPixelBufferGetBytesPerRowOfPlane(pixelBuffer, 0);
    if (!source || cropSize < 1 || quirc_resize(_decoder, cropSize, cropSize) < 0) {
        CVPixelBufferUnlockBaseAddress(pixelBuffer, kCVPixelBufferLock_ReadOnly);
        return;
    }
    int decoderWidth = 0, decoderHeight = 0;
    uint8_t *image = quirc_begin(_decoder, &decoderWidth, &decoderHeight);
    unsigned int histogram[256] = {0};
    unsigned int samples = 0;
    for (int y = 0; y < cropSize; y += 2) {
        uint8_t *row = source + (cropY + y) * stride + cropX;
        for (int x = 0; x < cropSize; x += 2) {
            histogram[row[x]]++;
            samples++;
        }
    }
    unsigned int lowTarget = samples / 100, highTarget = samples - lowTarget;
    unsigned int cumulative = 0;
    int low = 0, high = 255;
    for (int value = 0; value < 256; value++) {
        cumulative += histogram[value];
        if (cumulative >= lowTarget) { low = value; break; }
    }
    cumulative = 0;
    for (int value = 0; value < 256; value++) {
        cumulative += histogram[value];
        if (cumulative >= highTarget) { high = value; break; }
    }
    for (int y = 0; y < cropSize; y++) {
        uint8_t *input = source + (cropY + y) * stride + cropX;
        uint8_t *output = image + y * decoderWidth;
        if (high - low < 24) {
            memcpy(output, input, (size_t)cropSize);
            continue;
        }
        for (int x = 0; x < cropSize; x++) {
            int value = ((int)input[x] - low) * 255 / (high - low);
            output[x] = (uint8_t)(value < 0 ? 0 : (value > 255 ? 255 : value));
        }
    }
    CVPixelBufferUnlockBaseAddress(pixelBuffer, kCVPixelBufferLock_ReadOnly);
    quirc_end(_decoder);
    for (int i = 0; i < quirc_count(_decoder); i++) {
        struct quirc_code code;
        struct quirc_data data;
        quirc_extract(_decoder, i, &code);
        if (quirc_decode(&code, &data) != QUIRC_SUCCESS || data.payload_len < 1) continue;
        NSString *value = [[NSString alloc] initWithBytes:data.payload length:data.payload_len encoding:NSUTF8StringEncoding];
        if (![value hasPrefix:@"surf://pair?"]) continue;
        self.delivered = YES;
        dispatch_async(dispatch_get_main_queue(), ^{
            [self.captureSession stopRunning];
            [self.delegate qrScanner:self didScanValue:value];
        });
        break;
    }
}

- (void)dealloc {
    [self.captureSession stopRunning];
    if (_decoder) quirc_destroy(_decoder);
#if !OS_OBJECT_USE_OBJC
    if (_decodeQueue) dispatch_release(_decodeQueue);
#endif
}

@end

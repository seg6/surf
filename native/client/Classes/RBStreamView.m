#import "RBStreamView.h"
#import "RBLog.h"
#import "RBProtocol.h"

#import <QuartzCore/QuartzCore.h>

#include <math.h>
#import <OpenGLES/ES2/gl.h>
#import <OpenGLES/ES2/glext.h>

#include <stdlib.h>

enum { kRBMotionRateSamples = 31 };

@interface RBStreamView () {
    CVPixelBufferRef _pendingBuffer;
    CVPixelBufferRef _currentBuffer;
    CVOpenGLESTextureCacheRef _textureCache;
    GLuint _framebuffer;
    GLuint _renderbuffer;
    GLuint _program;
    GLint _positionSlot;
    GLint _texCoordSlot;
    CFTimeInterval _motionPresentationTimes[kRBMotionRateSamples];
    NSUInteger _motionPresentationCount;
    NSUInteger _motionPresentationCursor;
}
@property(nonatomic, strong) EAGLContext *glContext;
@property(nonatomic, strong) CADisplayLink *videoDisplayLink;
@property(nonatomic, strong) RBFrameMetadata *pendingMetadata;
@property(nonatomic, assign) NSUInteger presentedFrames;
@property(nonatomic, assign) NSUInteger overwrittenVideoFrames;
@property(nonatomic, assign) CFTimeInterval lastPresentationAt;
@property(nonatomic, assign) double maximumPresentationGapMS;
@property(nonatomic, assign) double recentMaximumPresentationGapMS;
@property(nonatomic, assign) double motionPresentationFPS;
@property(nonatomic, assign) unsigned int lastPresentedSourceSequence;
@property(nonatomic, assign) NSUInteger motionEpoch;
@property(nonatomic, assign) NSUInteger lastPresentedMotionEpoch;
@property(nonatomic, assign) BOOL motionTracking;
@property(nonatomic, assign) CFTimeInterval motionUntil;
@end

static GLuint RBCompileShader(GLenum type, const char *source) {
    GLuint shader = glCreateShader(type);
    GLint length = (GLint)strlen(source);
    glShaderSource(shader, 1, &source, &length);
    glCompileShader(shader);
    GLint ok = 0;
    glGetShaderiv(shader, GL_COMPILE_STATUS, &ok);
    if (!ok) {
        glDeleteShader(shader);
        return 0;
    }
    return shader;
}

@implementation RBStreamView

+ (Class)layerClass { return [CAEAGLLayer class]; }

- (BOOL)setupGL {
    self.glContext = [[EAGLContext alloc] initWithAPI:kEAGLRenderingAPIOpenGLES2];
    if (!self.glContext || ![EAGLContext setCurrentContext:self.glContext]) return NO;
    CAEAGLLayer *layer = (CAEAGLLayer *)self.layer;
    layer.opaque = YES;
    layer.drawableProperties = @{kEAGLDrawablePropertyRetainedBacking: @NO,
                                 kEAGLDrawablePropertyColorFormat: kEAGLColorFormatRGBA8};
    glGenFramebuffers(1, &_framebuffer);
    glGenRenderbuffers(1, &_renderbuffer);
    glBindFramebuffer(GL_FRAMEBUFFER, _framebuffer);
    glBindRenderbuffer(GL_RENDERBUFFER, _renderbuffer);
    [self.glContext renderbufferStorage:GL_RENDERBUFFER fromDrawable:layer];
    glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, _renderbuffer);

    const char *vertex =
        "attribute vec2 position; attribute vec2 texCoord;"
        "varying vec2 uv;"
        "void main(){ gl_Position=vec4(position,0.0,1.0); uv=texCoord; }";
    const char *fragment =
        "precision mediump float; varying vec2 uv;"
        "uniform sampler2D luma; uniform sampler2D chroma;"
        "void main(){ float y=texture2D(luma,uv).r-0.0625;"
        "vec2 c=texture2D(chroma,uv).ra-vec2(0.5,0.5);"
        "gl_FragColor=vec4(1.1643*y+1.5958*c.y,"
        "1.1643*y-0.39173*c.x-0.81290*c.y,"
        "1.1643*y+2.017*c.x,1.0); }";
    GLuint vs = RBCompileShader(GL_VERTEX_SHADER, vertex);
    GLuint fs = RBCompileShader(GL_FRAGMENT_SHADER, fragment);
    if (!vs || !fs) return NO;
    _program = glCreateProgram();
    glAttachShader(_program, vs);
    glAttachShader(_program, fs);
    glLinkProgram(_program);
    glDeleteShader(vs);
    glDeleteShader(fs);
    GLint linked = 0;
    glGetProgramiv(_program, GL_LINK_STATUS, &linked);
    if (!linked) return NO;
    _positionSlot = glGetAttribLocation(_program, "position");
    _texCoordSlot = glGetAttribLocation(_program, "texCoord");
    CVReturn status = CVOpenGLESTextureCacheCreate(kCFAllocatorDefault, NULL,
                                                   self.glContext,
                                                   NULL, &_textureCache);
    return status == kCVReturnSuccess;
}

- (id)initWithFrame:(CGRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        self.backgroundColor = [UIColor blackColor];
        self.opaque = YES;
        self.multipleTouchEnabled = YES;
        [self setupGL];
    }
    return self;
}

- (void)layoutSubviews {
    [super layoutSubviews];
    if (!self.glContext) return;
    [EAGLContext setCurrentContext:self.glContext];
    glBindRenderbuffer(GL_RENDERBUFFER, _renderbuffer);
    [self.glContext renderbufferStorage:GL_RENDERBUFFER fromDrawable:(CAEAGLLayer *)self.layer];
    glBindFramebuffer(GL_FRAMEBUFFER, _framebuffer);
    glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, _renderbuffer);
}

- (void)startVideoDisplayLinkIfNeeded {
    if (self.videoDisplayLink || !self.videoActive || !self.window) return;
    self.videoDisplayLink = [CADisplayLink displayLinkWithTarget:self selector:@selector(displayVideoTick:)];
    self.videoDisplayLink.frameInterval = 1;
    [self.videoDisplayLink addToRunLoop:[NSRunLoop mainRunLoop] forMode:NSRunLoopCommonModes];
}

- (void)didMoveToWindow {
    [super didMoveToWindow];
    if (self.window) [self startVideoDisplayLinkIfNeeded];
    else {
        [self.videoDisplayLink invalidate];
        self.videoDisplayLink = nil;
    }
}

- (void)setVideoActive:(BOOL)videoActive {
    if (_videoActive == videoActive) return;
    _videoActive = videoActive;
    if (videoActive) [self startVideoDisplayLinkIfNeeded];
    else {
        [self.videoDisplayLink invalidate];
        self.videoDisplayLink = nil;
        if (_pendingBuffer) { CVPixelBufferRelease(_pendingBuffer); _pendingBuffer = NULL; }
        if (_currentBuffer) { CVPixelBufferRelease(_currentBuffer); _currentBuffer = NULL; }
        self.pendingMetadata = nil;
        self.lastPresentationAt = 0.0;
        self.lastPresentedSourceSequence = 0;
        self.lastPresentedMotionEpoch = 0;
        self.motionTracking = NO;
        self.motionUntil = 0.0;
        _motionPresentationCount = 0;
        _motionPresentationCursor = 0;
        self.motionPresentationFPS = 0.0;
    }
}

- (void)beginMotionWindow {
    self.motionEpoch++;
    if (self.motionEpoch == 0) self.motionEpoch = 1;
    self.lastPresentedMotionEpoch = 0;
    self.motionTracking = YES;
    self.motionUntil = 0.0;
    self.maximumPresentationGapMS = 0.0;
    self.recentMaximumPresentationGapMS = 0.0;
    _motionPresentationCount = 0;
    _motionPresentationCursor = 0;
}

- (void)continueMotionWindow {
    if (self.motionEpoch == 0) [self beginMotionWindow];
    self.motionTracking = YES;
}

- (void)endMotionWindow {
    self.motionTracking = NO;
    // Client inertia has already ended. Allow only the final in-flight
    // capture/encode/decode frame to land; a later static-page update is not
    // a scroll hitch and must not inflate the motion gap.
    self.motionUntil = CACurrentMediaTime() + 0.12;
}

- (void)displayVideoPixelBuffer:(CVPixelBufferRef)pixelBuffer metadata:(RBFrameMetadata *)metadata {
    if (!pixelBuffer) return;
    CVPixelBufferRetain(pixelBuffer);
    if (_pendingBuffer) {
        self.overwrittenVideoFrames++;
        CVPixelBufferRelease(_pendingBuffer);
    }
    _pendingBuffer = pixelBuffer;
    self.pendingMetadata = metadata;
}

static unsigned char RBClampByte(int value) {
    return (unsigned char)(value < 0 ? 0 : (value > 255 ? 255 : value));
}

- (UIImage *)snapshotImageWithMaximumSize:(CGSize)maximumSize {
    if (!_currentBuffer || maximumSize.width < 1.0 || maximumSize.height < 1.0 ||
        CVPixelBufferGetPlaneCount(_currentBuffer) < 2) return nil;
    CVPixelBufferRetain(_currentBuffer);
    CVPixelBufferRef buffer = _currentBuffer;
    if (CVPixelBufferLockBaseAddress(buffer, 0) != kCVReturnSuccess) {
        CVPixelBufferRelease(buffer);
        return nil;
    }
    size_t sourceW = CVPixelBufferGetWidthOfPlane(buffer, 0);
    size_t sourceH = CVPixelBufferGetHeightOfPlane(buffer, 0);
    double scale = MIN(1.0, MIN(maximumSize.width / MAX(1.0, (double)sourceW),
                                maximumSize.height / MAX(1.0, (double)sourceH)));
    size_t outputW = MAX(1, (size_t)floor(sourceW * scale));
    size_t outputH = MAX(1, (size_t)floor(sourceH * scale));
    size_t outputStride = outputW * 4;
    unsigned char *pixels = (unsigned char *)calloc(outputH, outputStride);
    unsigned char *yPlane = (unsigned char *)CVPixelBufferGetBaseAddressOfPlane(buffer, 0);
    unsigned char *uvPlane = (unsigned char *)CVPixelBufferGetBaseAddressOfPlane(buffer, 1);
    size_t yStride = CVPixelBufferGetBytesPerRowOfPlane(buffer, 0);
    size_t uvStride = CVPixelBufferGetBytesPerRowOfPlane(buffer, 1);
    if (!pixels || !yPlane || !uvPlane) {
        free(pixels);
        CVPixelBufferUnlockBaseAddress(buffer, 0);
        CVPixelBufferRelease(buffer);
        return nil;
    }
    {
        for (size_t y = 0; y < outputH; y++) {
            size_t sourceY = MIN(sourceH - 1, y * sourceH / outputH);
            unsigned char *output = pixels + y * outputStride;
            unsigned char *sourceLuma = yPlane + sourceY * yStride;
            unsigned char *sourceChroma = uvPlane + (sourceY / 2) * uvStride;
            for (size_t x = 0; x < outputW; x++) {
                size_t sourceX = MIN(sourceW - 1, x * sourceW / outputW);
                int c = MAX(0, (int)sourceLuma[sourceX] - 16);
                size_t uvX = (sourceX / 2) * 2;
                int d = (int)sourceChroma[uvX] - 128;
                int e = (int)sourceChroma[uvX + 1] - 128;
                output[x * 4] = RBClampByte((298 * c + 409 * e + 128) >> 8);
                output[x * 4 + 1] = RBClampByte((298 * c - 100 * d - 208 * e + 128) >> 8);
                output[x * 4 + 2] = RBClampByte((298 * c + 516 * d + 128) >> 8);
                output[x * 4 + 3] = 255;
            }
        }
    }
    CVPixelBufferUnlockBaseAddress(buffer, 0);
    CVPixelBufferRelease(buffer);
    CGColorSpaceRef colorSpace = CGColorSpaceCreateDeviceRGB();
    CGContextRef context = CGBitmapContextCreate(pixels, outputW, outputH, 8, outputStride,
                                                  colorSpace,
                                                  kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big);
    CGColorSpaceRelease(colorSpace);
    if (!context) {
        free(pixels);
        return nil;
    }
    CGImageRef cgImage = CGBitmapContextCreateImage(context);
    UIImage *image = cgImage ? [UIImage imageWithCGImage:cgImage] : nil;
    if (cgImage) CGImageRelease(cgImage);
    CGContextRelease(context);
    free(pixels);
    return image;
}

- (BOOL)drawPixelBuffer:(CVPixelBufferRef)pixelBuffer {
    if (!pixelBuffer || !_textureCache || !self.window ||
        [UIApplication sharedApplication].applicationState != UIApplicationStateActive ||
        CVPixelBufferGetPlaneCount(pixelBuffer) < 2) return NO;
    if (![EAGLContext setCurrentContext:self.glContext]) return NO;
    CVOpenGLESTextureRef yTexture = NULL;
    CVOpenGLESTextureRef uvTexture = NULL;
    size_t width = CVPixelBufferGetWidthOfPlane(pixelBuffer, 0);
    size_t height = CVPixelBufferGetHeightOfPlane(pixelBuffer, 0);
    size_t chromaWidth = CVPixelBufferGetWidthOfPlane(pixelBuffer, 1);
    size_t chromaHeight = CVPixelBufferGetHeightOfPlane(pixelBuffer, 1);
    CVReturn yStatus = CVOpenGLESTextureCacheCreateTextureFromImage(kCFAllocatorDefault, _textureCache,
        pixelBuffer, NULL, GL_TEXTURE_2D, GL_LUMINANCE, (GLsizei)width, (GLsizei)height,
        GL_LUMINANCE, GL_UNSIGNED_BYTE, 0, &yTexture);
    CVReturn uvStatus = CVOpenGLESTextureCacheCreateTextureFromImage(kCFAllocatorDefault, _textureCache,
        pixelBuffer, NULL, GL_TEXTURE_2D, GL_LUMINANCE_ALPHA, (GLsizei)chromaWidth, (GLsizei)chromaHeight,
        GL_LUMINANCE_ALPHA, GL_UNSIGNED_BYTE, 1, &uvTexture);
    if (yStatus != kCVReturnSuccess || uvStatus != kCVReturnSuccess) {
        if (yTexture) CFRelease(yTexture);
        if (uvTexture) CFRelease(uvTexture);
        return NO;
    }
    glBindFramebuffer(GL_FRAMEBUFFER, _framebuffer);
    if (glCheckFramebufferStatus(GL_FRAMEBUFFER) != GL_FRAMEBUFFER_COMPLETE) {
        CFRelease(yTexture);
        CFRelease(uvTexture);
        return NO;
    }
    glViewport(0, 0, (GLsizei)(self.bounds.size.width * self.contentScaleFactor),
               (GLsizei)(self.bounds.size.height * self.contentScaleFactor));
    glClearColor(0, 0, 0, 1);
    glClear(GL_COLOR_BUFFER_BIT);
    glUseProgram(_program);
    const GLfloat vertices[] = {-1,-1, 1,-1, -1,1, 1,1};
    const GLfloat texCoords[] = {0,1, 1,1, 0,0, 1,0};
    glVertexAttribPointer(_positionSlot, 2, GL_FLOAT, GL_FALSE, 0, vertices);
    glEnableVertexAttribArray(_positionSlot);
    glVertexAttribPointer(_texCoordSlot, 2, GL_FLOAT, GL_FALSE, 0, texCoords);
    glEnableVertexAttribArray(_texCoordSlot);
    glActiveTexture(GL_TEXTURE0);
    glBindTexture(CVOpenGLESTextureGetTarget(yTexture), CVOpenGLESTextureGetName(yTexture));
    // CVOpenGLESTexture does not promise non-mipmapped sampling defaults on
    // the PowerVR SGX. Without these parameters the planes are incomplete;
    // sampling the UV texture returns (0,0,0,1), which our YUV matrix renders
    // as a solid red frame.
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
    glUniform1i(glGetUniformLocation(_program, "luma"), 0);
    glActiveTexture(GL_TEXTURE1);
    glBindTexture(CVOpenGLESTextureGetTarget(uvTexture), CVOpenGLESTextureGetName(uvTexture));
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
    glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
    glUniform1i(glGetUniformLocation(_program, "chroma"), 1);
    glDrawArrays(GL_TRIANGLE_STRIP, 0, 4);
    // iOS 6's SGX543 driver can return from present while it still consumes
    // CoreVideo-backed textures. Releasing and flushing that cache immediately
    // afterward then produces a recurring crash in presentRenderbuffer. Finish
    // the draw and unbind both planes before presentation so their lifetime is
    // unambiguous even on the legacy driver.
    glFinish();
    GLenum drawError = glGetError();
    glActiveTexture(GL_TEXTURE1);
    glBindTexture(GL_TEXTURE_2D, 0);
    glActiveTexture(GL_TEXTURE0);
    glBindTexture(GL_TEXTURE_2D, 0);
    glBindRenderbuffer(GL_RENDERBUFFER, _renderbuffer);
    BOOL presented = drawError == GL_NO_ERROR &&
        [self.glContext presentRenderbuffer:GL_RENDERBUFFER];
    if (!presented) RBLog(@"video: OpenGL draw failed error=0x%x", drawError);
    CFRelease(yTexture);
    CFRelease(uvTexture);
    CVOpenGLESTextureCacheFlush(_textureCache, 0);
    return presented;
}

- (void)displayVideoTick:(CADisplayLink *)displayLink {
    BOOL newFrame = _pendingBuffer != NULL;
    if (!self.videoActive || !newFrame) return;
    RBFrameMetadata *metadata = nil;
    if (newFrame) {
        if (_currentBuffer) CVPixelBufferRelease(_currentBuffer);
        _currentBuffer = _pendingBuffer;
        _pendingBuffer = NULL;
        metadata = self.pendingMetadata;
        self.pendingMetadata = nil;
    }
    if (![self drawPixelBuffer:_currentBuffer]) return;
    CFTimeInterval now = CACurrentMediaTime();
    BOOL inMotionWindow = self.motionTracking || now <= self.motionUntil;
    if (inMotionWindow && self.lastPresentedMotionEpoch == self.motionEpoch &&
        self.lastPresentationAt > 0.0) {
        double gapMS = (now - self.lastPresentationAt) * 1000.0;
        if (gapMS > self.maximumPresentationGapMS) self.maximumPresentationGapMS = gapMS;
        if (gapMS > self.recentMaximumPresentationGapMS) self.recentMaximumPresentationGapMS = gapMS;
    }
    if (inMotionWindow) {
        _motionPresentationTimes[_motionPresentationCursor] = now;
        _motionPresentationCursor = (_motionPresentationCursor + 1) % kRBMotionRateSamples;
        if (_motionPresentationCount < kRBMotionRateSamples) _motionPresentationCount++;
        if (_motionPresentationCount >= 2) {
            NSUInteger oldest = (_motionPresentationCursor + kRBMotionRateSamples -
                                 _motionPresentationCount) % kRBMotionRateSamples;
            NSUInteger newest = (_motionPresentationCursor + kRBMotionRateSamples - 1) %
                                kRBMotionRateSamples;
            CFTimeInterval elapsed = _motionPresentationTimes[newest] -
                                     _motionPresentationTimes[oldest];
            if (elapsed > 0.0) {
                self.motionPresentationFPS = (_motionPresentationCount - 1) / elapsed;
            }
        }
    }
    self.lastPresentedMotionEpoch = inMotionWindow ? self.motionEpoch : 0;
    self.lastPresentationAt = now;
    self.lastPresentedSourceSequence = metadata.sourceSequence;
    self.presentedFrames++;
    [self.presentationDelegate streamView:self didPresentMetadata:metadata];
}

- (double)consumeRecentMaximumPresentationGapMS {
    double value = self.recentMaximumPresentationGapMS;
    self.recentMaximumPresentationGapMS = 0.0;
    return value;
}

- (void)dealloc {
    [self.videoDisplayLink invalidate];
    if (_pendingBuffer) CVPixelBufferRelease(_pendingBuffer);
    if (_currentBuffer) CVPixelBufferRelease(_currentBuffer);
    if (_textureCache) CFRelease(_textureCache);
    if (_program) glDeleteProgram(_program);
    if (_framebuffer) glDeleteFramebuffers(1, &_framebuffer);
    if (_renderbuffer) glDeleteRenderbuffers(1, &_renderbuffer);
    if ([EAGLContext currentContext] == self.glContext) [EAGLContext setCurrentContext:nil];
}

@end

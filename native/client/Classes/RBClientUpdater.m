#import "RBClientUpdater.h"
#import "RBLog.h"
#import <CommonCrypto/CommonDigest.h>
#import <errno.h>
#import <spawn.h>
#import <sys/wait.h>

extern char **environ;

static NSString *const RBUpdateResultPath = @"/var/mobile/Library/Surf/update-result";

@interface RBClientUpdater ()
@property(nonatomic, strong) NSURL *baseURL;
@property(nonatomic, strong) NSDictionary *update;
@property(nonatomic, strong) NSURLConnection *connection;
@property(nonatomic, strong) NSFileHandle *file;
@property(nonatomic, copy) NSString *path;
@property(nonatomic, assign) long long received;
@end

@implementation RBClientUpdater

- (id)initWithBaseURL:(NSURL *)baseURL update:(NSDictionary *)update {
    self = [super init];
    if (self) {
        self.baseURL = baseURL;
        self.update = update;
    }
    return self;
}

- (void)start {
    NSString *directory = @"/var/mobile/Library/Surf";
    [[NSFileManager defaultManager] createDirectoryAtPath:directory
                              withIntermediateDirectories:YES attributes:nil error:nil];
    self.path = [directory stringByAppendingPathComponent:@"update.deb"];
    [[NSFileManager defaultManager] removeItemAtPath:self.path error:nil];
    if (![[NSFileManager defaultManager] createFileAtPath:self.path contents:nil attributes:nil]) {
        [self fail:@"Could not create the update file"];
        return;
    }
    self.file = [NSFileHandle fileHandleForWritingAtPath:self.path];
    NSString *relative = [self.update objectForKey:@"url"];
    NSURL *url = [NSURL URLWithString:relative relativeToURL:self.baseURL];
    NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:url
                                                           cachePolicy:NSURLRequestReloadIgnoringLocalCacheData
                                                       timeoutInterval:60.0];
    self.connection = [[NSURLConnection alloc] initWithRequest:request delegate:self startImmediately:YES];
}

- (void)connection:(NSURLConnection *)connection didReceiveResponse:(NSURLResponse *)response {
    if ([(NSHTTPURLResponse *)response statusCode] != 200) {
        [connection cancel];
        [self fail:[NSString stringWithFormat:@"Update download returned HTTP %d",
                    (int)[(NSHTTPURLResponse *)response statusCode]]];
    }
}

- (void)connection:(NSURLConnection *)connection didReceiveData:(NSData *)data {
    [self.file writeData:data];
    self.received += [data length];
    long long size = [[self.update objectForKey:@"size"] longLongValue];
    if (size > 0) {
        [self.delegate clientUpdater:self progress:MIN(1.0, (double)self.received / (double)size)];
    }
}

- (void)connection:(NSURLConnection *)connection didFailWithError:(NSError *)error {
    [self.file closeFile];
    self.file = nil;
    [self fail:[error localizedDescription] ?: @"Update download failed"];
}

- (void)connectionDidFinishLoading:(NSURLConnection *)connection {
    [self.file synchronizeFile];
    [self.file closeFile];
    self.file = nil;
    long long wantedSize = [[self.update objectForKey:@"size"] longLongValue];
    if (wantedSize <= 0 || self.received != wantedSize) {
        [self fail:@"The downloaded update has the wrong size"];
        return;
    }
    NSString *wantedHash = [[self.update objectForKey:@"sha256"] lowercaseString];
    NSString *actualHash = [self sha256AtPath:self.path];
    if (![actualHash isEqualToString:wantedHash]) {
        [self fail:@"The downloaded update failed its checksum"];
        return;
    }
    [self installWithHash:wantedHash version:[self.update objectForKey:@"version"]];
}

- (NSString *)sha256AtPath:(NSString *)path {
    NSFileHandle *input = [NSFileHandle fileHandleForReadingAtPath:path];
    if (!input) return nil;
    CC_SHA256_CTX context;
    CC_SHA256_Init(&context);
    for (;;) {
        @autoreleasepool {
            NSData *chunk = [input readDataOfLength:64 * 1024];
            if (![chunk length]) break;
            CC_SHA256_Update(&context, [chunk bytes], (CC_LONG)[chunk length]);
        }
    }
    [input closeFile];
    unsigned char digest[CC_SHA256_DIGEST_LENGTH];
    CC_SHA256_Final(digest, &context);
    NSMutableString *result = [NSMutableString stringWithCapacity:CC_SHA256_DIGEST_LENGTH * 2];
    for (NSUInteger i = 0; i < CC_SHA256_DIGEST_LENGTH; i++) [result appendFormat:@"%02x", digest[i]];
    return result;
}

- (void)installWithHash:(NSString *)hash version:(NSString *)version {
    NSString *helper = @"/usr/libexec/surf-update-v2";
    if (![[NSFileManager defaultManager] isExecutableFileAtPath:helper]) {
        [self fail:@"This Surf build needs one final manual update before in-app updates are available"];
        return;
    }
    [[NSFileManager defaultManager] removeItemAtPath:RBUpdateResultPath error:nil];
    dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
        pid_t pid = 0;
        const char *argv[] = {
            [helper fileSystemRepresentation], [self.path fileSystemRepresentation],
            [hash UTF8String], [version UTF8String], NULL
        };
        int spawnError = posix_spawn(&pid, argv[0], NULL, NULL, (char *const *)argv, environ);
        int status = 0;
        int waitError = 0;
        if (spawnError == 0 && waitpid(pid, &status, 0) < 0) waitError = errno;
        NSDictionary *result = [self installResult];
        BOOL recordedSuccess =
            [[result objectForKey:@"schema"] isEqualToString:@"1"] &&
            [[result objectForKey:@"stage"] isEqualToString:@"complete"] &&
            [[result objectForKey:@"result"] intValue] == 0 &&
            [[result objectForKey:@"version"] isEqualToString:version] &&
            [[[result objectForKey:@"sha256"] lowercaseString] isEqualToString:[hash lowercaseString]];
        int exitCode = WIFEXITED(status) ? WEXITSTATUS(status) : -1;
        RBLog(@"client update installer spawn=%d wait=%d exited=%d exit=%d record=%@",
              spawnError, waitError, WIFEXITED(status), exitCode, result ?: @{});
        dispatch_async(dispatch_get_main_queue(), ^{
            if (recordedSuccess || (spawnError == 0 && waitError == 0 &&
                                    WIFEXITED(status) && exitCode == 0)) {
                [self.delegate clientUpdaterDidInstall:self];
            } else {
                NSString *stage = [result objectForKey:@"stage"];
                int code = [[result objectForKey:@"result"] intValue];
                if (![stage length]) stage = spawnError ? @"launch" : (waitError ? @"wait" : @"installer");
                if (code == 0) code = spawnError ?: (waitError ?: exitCode);
                [self fail:[NSString stringWithFormat:@"Installer failed during %@ (%d)", stage, code]];
            }
        });
    });
}

- (NSDictionary *)installResult {
    NSString *contents = [NSString stringWithContentsOfFile:RBUpdateResultPath
                                                   encoding:NSUTF8StringEncoding error:nil];
    if (![contents length]) return nil;
    NSMutableDictionary *result = [NSMutableDictionary dictionary];
    for (NSString *line in [contents componentsSeparatedByCharactersInSet:
                            [NSCharacterSet newlineCharacterSet]]) {
        NSRange separator = [line rangeOfString:@"="];
        if (separator.location == NSNotFound || separator.location == 0) continue;
        NSString *key = [line substringToIndex:separator.location];
        NSString *value = [line substringFromIndex:separator.location + 1];
        [result setObject:value forKey:key];
    }
    return result;
}

- (void)fail:(NSString *)message {
    RBLog(@"client update failed: %@", message);
    [[NSFileManager defaultManager] removeItemAtPath:self.path error:nil];
    [self.delegate clientUpdater:self failed:message ?: @"Update failed"];
}

@end

#import "RBAppDelegate.h"
#import "RBLog.h"
#import "RBRootViewController.h"

@implementation RBAppDelegate

- (BOOL)application:(UIApplication *)application didFinishLaunchingWithOptions:(NSDictionary *)launchOptions {
    RBInstallCrashHandlers();
    RBLog(@"application launching");

    self.window = [[UIWindow alloc] initWithFrame:[[UIScreen mainScreen] bounds]];
    self.window.rootViewController = [[RBRootViewController alloc] init];
    [self.window makeKeyAndVisible];
    return YES;
}

- (RBRootViewController *)rootController {
    UIViewController *root = self.window.rootViewController;
    return [root isKindOfClass:[RBRootViewController class]] ? (RBRootViewController *)root : nil;
}

// surf:<url>, surf://<url>, surf-http(s)://host/… — other apps open links in
// Surf (M4.1). surf-http rewrites are the scheme-swap convention so plain
// links can be retargeted by prefixing.
- (BOOL)application:(UIApplication *)application openURL:(NSURL *)url
  sourceApplication:(NSString *)sourceApplication annotation:(id)annotation {
    NSString *raw = [url absoluteString];
    NSString *target = nil;
    if ([raw hasPrefix:@"surf-http://"]) {
        target = [@"http://" stringByAppendingString:[raw substringFromIndex:[@"surf-http://" length]]];
    } else if ([raw hasPrefix:@"surf-https://"]) {
        target = [@"https://" stringByAppendingString:[raw substringFromIndex:[@"surf-https://" length]]];
    } else if ([raw hasPrefix:@"surf:"]) {
        target = [raw substringFromIndex:[@"surf:" length]];
        while ([target hasPrefix:@"/"]) target = [target substringFromIndex:1];
        target = [target stringByReplacingPercentEscapesUsingEncoding:NSUTF8StringEncoding] ?: target;
        if ([target length] && ![target hasPrefix:@"http://"] && ![target hasPrefix:@"https://"]) {
            target = [@"https://" stringByAppendingString:target];
        }
    }
    if (![target length]) return NO;
    RBLog(@"open-url %@ -> %@", raw, target);
    [[self rootController] openURLString:target];
    return YES;
}

- (void)applicationDidBecomeActive:(UIApplication *)application {
    // "Open copied link?" (M4.2)
    [[self rootController] checkPasteboard];
}

- (void)applicationDidReceiveMemoryWarning:(UIApplication *)application {
    RBLog(@"memory warning");
}

- (void)applicationWillTerminate:(UIApplication *)application {
    RBLog(@"application terminating");
}

@end

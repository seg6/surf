#import "RBAppDelegate.h"
#import "RBConfig.h"
#import "RBLog.h"
#import "RBRootViewController.h"
#import "RBTheme.h"

@implementation RBAppDelegate

- (BOOL)application:(UIApplication *)application didFinishLaunchingWithOptions:(NSDictionary *)launchOptions {
    RBInstallCrashHandlers();
    RBLogEvent(@"application", @"info", @{@"state": @"launching"}, @"Application launching");

    // Mobile pages are the natural default on the touch-first Surf client.
    // registerDefaults preserves an explicit user choice on upgrades.
    [[NSUserDefaults standardUserDefaults] registerDefaults:@{
        RBDefaultsMobileLayoutKey: @YES,
        RBDefaultsDarkModeKey: @NO,
        RBDefaultsBottomBrowserBarKey: @YES
    }];
    self.window = [[UIWindow alloc] initWithFrame:[[UIScreen mainScreen] bounds]];
    self.window.rootViewController = [[RBRootViewController alloc] init];
    [self.window makeKeyAndVisible];
    NSURL *launchURL = [launchOptions objectForKey:UIApplicationLaunchOptionsURLKey];
    if (launchURL) {
        dispatch_async(dispatch_get_main_queue(), ^{
            [self application:application openURL:launchURL sourceApplication:nil annotation:nil];
        });
    }
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
    if ([[[url scheme] lowercaseString] isEqualToString:@"surf"] && [[[url host] lowercaseString] isEqualToString:@"pair"]) {
        [[self rootController] openPairingURL:url];
        return YES;
    } else if ([raw hasPrefix:@"surf-http://"]) {
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
    RBLogEvent(@"application", @"info", @{@"action": @"open_url", @"target": target ?: @""}, @"External URL opened");
    [[self rootController] openURLString:target];
    return YES;
}

- (void)applicationDidBecomeActive:(UIApplication *)application {
    [[self rootController] applicationDidBecomeActive];
    // "Open copied link?" (M4.2)
    [[self rootController] checkPasteboard];
}

- (void)applicationDidEnterBackground:(UIApplication *)application {
    [[self rootController] syncNativeLog];
    [[self rootController] applicationDidEnterBackground];
}

- (void)applicationDidReceiveMemoryWarning:(UIApplication *)application {
    RBLogEvent(@"application", @"warn", @{@"event": @"memory_warning"}, @"Memory warning received");
}

- (void)applicationWillTerminate:(UIApplication *)application {
    RBLogEvent(@"application", @"info", @{@"state": @"terminating"}, @"Application terminating");
}

@end

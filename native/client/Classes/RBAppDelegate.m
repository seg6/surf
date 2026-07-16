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

- (void)applicationDidReceiveMemoryWarning:(UIApplication *)application {
    RBLog(@"memory warning");
}

- (void)applicationWillTerminate:(UIApplication *)application {
    RBLog(@"application terminating");
}

@end

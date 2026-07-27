#import <Foundation/Foundation.h>

#ifndef RBNativeVersion
#error RBNativeVersion must be supplied by native/client/Makefile from PROTOCOL_VERSION
#endif
#ifndef RBAppVersion
#error RBAppVersion must be supplied by native/client/Makefile from VERSION
#endif
#define RBDefaultServerURL @"https://surf.seg6.space"
#define RBDefaultPassword @""
#define RBLogDirectory @"/var/mobile/Library/Surf"
#define RBLogFile @"/var/mobile/Library/Surf/surf.log"

// NSUserDefaults keys (settings screen).
#define RBDefaultsServerURLKey @"RBServerURL"
#define RBDefaultsPasswordKey @"RBPassword"
#define RBDefaultsServersKey @"RBServers" // [{title,url,password?}]
#define RBDefaultsLastPasteboardKey @"RBLastPasteboard" // last URL offered from the pasteboard
#define RBDefaultsReaderNightKey @"RBReaderNight" // NSNumber bool; reader dark mode
#define RBDefaultsDiagnosticsKey @"RBDiagnosticsOverlay" // NSNumber bool; live performance panel

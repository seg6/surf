#import <Foundation/Foundation.h>

#ifndef RBNativeVersion
#error RBNativeVersion must be supplied by native/client/Makefile from PROTOCOL_VERSION
#endif
#ifndef RBAppVersion
#error RBAppVersion must be supplied by native/client/Makefile from VERSION
#endif
#define RBLogDirectory @"/var/mobile/Library/Surf"
#define RBLogFile @"/var/mobile/Library/Surf/surf.log"

// NSUserDefaults keys (settings screen).
#define RBLegacyDefaultsServerURLKey @"RBServerURL"
#define RBLegacyDefaultsPasswordKey @"RBPassword"
#define RBLegacyDefaultsServersKey @"RBServers"
#define RBDefaultsLastPasteboardKey @"RBLastPasteboard" // last URL offered from the pasteboard
#define RBDefaultsReaderNightKey @"RBReaderNight" // NSNumber bool; reader dark mode
#define RBDefaultsDiagnosticsKey @"RBDiagnosticsOverlay" // NSNumber bool; live performance panel
#define RBDefaultsMobileLayoutKey @"RBMobileLayout" // NSNumber bool; request mobile sites and viewport behavior
#define RBDefaultsOfferCopiedLinksKey @"RBOfferCopiedLinks" // NSNumber bool; prompt for clipboard URLs
#define RBDefaultsDarkModeKey @"RBDarkMode" // NSNumber bool; native UI and remote Chromium appearance
#define RBDefaultsBottomBrowserBarKey @"RBBottomBrowserBar" // NSNumber bool; iPad browser rail edge

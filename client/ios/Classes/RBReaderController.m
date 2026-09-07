#import "RBReaderController.h"
#import "RBConfig.h"
#import "RBTheme.h"

@interface RBReaderController () <UIWebViewDelegate>
@property(nonatomic, copy) NSString *articleTitle;
@property(nonatomic, copy) NSString *articleHTML;
@property(nonatomic, copy) NSString *articleURL;
@property(nonatomic, strong) UIWebView *webView;
@property(nonatomic, strong) UINavigationBar *navBar;
@property(nonatomic, assign) BOOL night;
@end

@implementation RBReaderController

- (id)initWithTitle:(NSString *)title html:(NSString *)html url:(NSString *)url {
    self = [super init];
    if (self) {
        _articleTitle = [title copy];
        _articleHTML = [html copy];
        _articleURL = [url copy];
        _night = [RBTheme isDarkMode] ||
                 [[[NSUserDefaults standardUserDefaults] objectForKey:RBDefaultsReaderNightKey] boolValue];
        self.modalPresentationStyle = UIModalPresentationFullScreen;
        self.modalTransitionStyle = UIModalTransitionStyleCoverVertical;
    }
    return self;
}

- (void)viewDidLoad {
    [super viewDidLoad];
    self.view.backgroundColor = [RBTheme pageBackgroundColor];

    self.navBar = [[UINavigationBar alloc] initWithFrame:CGRectMake(0.0, 0.0, self.view.bounds.size.width, 44.0)];
    self.navBar.autoresizingMask = UIViewAutoresizingFlexibleWidth;
    [RBTheme styleNavigationBar:self.navBar];
    UINavigationItem *item = [[UINavigationItem alloc] initWithTitle:self.articleTitle ?: @"Reader"];
    item.leftBarButtonItem = [[UIBarButtonItem alloc] initWithBarButtonSystemItem:UIBarButtonSystemItemDone
                                                                           target:self action:@selector(doneTapped:)];
    UIButton *nightButton = [RBTheme barButtonWithIcon:RBIconMoon target:self action:@selector(nightTapped:)];
    nightButton.frame = CGRectMake(0.0, 0.0, 42.0, 36.0);
    [nightButton setImage:[RBTheme icon:RBIconMoon size:20.0 color:[UIColor whiteColor]]
                 forState:UIControlStateNormal];
    [nightButton setImage:[RBTheme icon:RBIconMoon size:20.0 color:[RBTheme seaGlassColor]]
                 forState:UIControlStateHighlighted];
    nightButton.accessibilityLabel = @"Toggle night mode";
    item.rightBarButtonItem = [[UIBarButtonItem alloc] initWithCustomView:nightButton];
    [self.navBar pushNavigationItem:item animated:NO];
    [self.view addSubview:self.navBar];

    self.webView = [[UIWebView alloc] initWithFrame:CGRectMake(0.0, 44.0, self.view.bounds.size.width,
                                                               self.view.bounds.size.height - 44.0)];
    self.webView.autoresizingMask = UIViewAutoresizingFlexibleWidth | UIViewAutoresizingFlexibleHeight;
    self.webView.delegate = self;
    self.webView.scalesPageToFit = NO;
    [self.view addSubview:self.webView];
    [self render];
}

- (void)render {
    NSString *bg = self.night ? @"#0f2a3a" : @"#f7fafc";
    NSString *fg = self.night ? @"#dcecf3" : @"#102a3a";
    NSString *link = self.night ? @"#63c2cf" : @"#1473b8";
    NSString *page = [NSString stringWithFormat:
        @"<!doctype html><html><head><meta charset=\"utf-8\">"
        @"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">"
        @"<style>"
        @"body{background:%@;color:%@;font-family:Georgia,serif;font-size:19px;line-height:1.55;"
        @"margin:0;padding:20px 24px 60px;}"
        @"#rb-article{max-width:640px;margin:0 auto;}"
        @"h1.rb-title{font-size:26px;line-height:1.25;margin:0 0 4px;}"
        @".rb-src{color:#5d7280;font-size:13px;font-family:Helvetica,sans-serif;margin:0 0 24px;"
        @"border-bottom:1px solid rgba(128,128,128,0.25);padding-bottom:14px;}"
        @"img{max-width:100%%;height:auto;}"
        @"a{color:%@;}"
        @"pre{overflow-x:auto;font-size:14px;background:rgba(128,128,128,0.12);padding:10px;}"
        @"table{max-width:100%%;overflow-x:auto;display:block;}"
        @"</style></head><body><div id=\"rb-article\">"
        @"<h1 class=\"rb-title\">%@</h1><div class=\"rb-src\">%@</div>%@"
        @"</div></body></html>",
        bg, fg, link,
        [self escapeHTML:self.articleTitle ?: @""],
        [self escapeHTML:self.articleURL ?: @""],
        self.articleHTML ?: @""];
    [self.webView loadHTMLString:page baseURL:[NSURL URLWithString:self.articleURL ?: @""]];
}

- (NSString *)escapeHTML:(NSString *)text {
    NSString *out = [text stringByReplacingOccurrencesOfString:@"&" withString:@"&amp;"];
    out = [out stringByReplacingOccurrencesOfString:@"<" withString:@"&lt;"];
    return [out stringByReplacingOccurrencesOfString:@">" withString:@"&gt;"];
}

// Links inside the article leave reader mode and navigate the remote page.
- (BOOL)webView:(UIWebView *)webView shouldStartLoadWithRequest:(NSURLRequest *)request
                                                 navigationType:(UIWebViewNavigationType)navigationType {
    if (navigationType == UIWebViewNavigationTypeLinkClicked) {
        NSString *url = [[request URL] absoluteString];
        void (^dismiss)(void) = self.onDismiss;
        [self.presentingViewController dismissViewControllerAnimated:YES completion:^{
            if (dismiss) dismiss();
            // The root VC re-enters stream mode; it also receives this URL.
            [[NSNotificationCenter defaultCenter] postNotificationName:@"RBReaderNavigate" object:url];
        }];
        return NO;
    }
    return YES;
}

- (void)nightTapped:(id)sender {
    self.night = !self.night;
    [[NSUserDefaults standardUserDefaults] setObject:[NSNumber numberWithBool:self.night]
                                              forKey:RBDefaultsReaderNightKey];
    [[NSUserDefaults standardUserDefaults] synchronize];
    [self render];
}

- (void)doneTapped:(id)sender {
    void (^dismiss)(void) = self.onDismiss;
    [self.presentingViewController dismissViewControllerAnimated:YES completion:^{
        if (dismiss) dismiss();
    }];
}

@end

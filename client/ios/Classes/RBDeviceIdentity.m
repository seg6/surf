#import "RBDeviceIdentity.h"
#import "RBLog.h"
#import <CommonCrypto/CommonDigest.h>

static NSString *RBBase64URL(NSData *data) {
    static const char table[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";
    const unsigned char *bytes = (const unsigned char *)[data bytes];
    NSUInteger length = [data length];
    NSMutableString *result = [NSMutableString stringWithCapacity:((length + 2) / 3) * 4];
    for (NSUInteger i = 0; i < length; i += 3) {
        unsigned int value = (unsigned int)bytes[i] << 16;
        BOOL second = i + 1 < length, third = i + 2 < length;
        if (second) value |= (unsigned int)bytes[i + 1] << 8;
        if (third) value |= bytes[i + 2];
        [result appendFormat:@"%c%c", table[(value >> 18) & 63], table[(value >> 12) & 63]];
        if (second) [result appendFormat:@"%c", table[(value >> 6) & 63]];
        if (third) [result appendFormat:@"%c", table[value & 63]];
    }
    return result;
}

static NSError *RBKeyError(OSStatus status, NSString *message) {
    return [NSError errorWithDomain:@"SurfKeychain" code:status userInfo:@{NSLocalizedDescriptionKey: message ?: @"Keychain error"}];
}

@implementation RBDeviceIdentity

+ (NSString *)keyTagForServerID:(NSString *)serverID {
    return [@"space.seg6.surf.device." stringByAppendingString:[serverID lowercaseString]];
}

+ (NSData *)tagData:(NSString *)serverID {
    return [[self keyTagForServerID:serverID] dataUsingEncoding:NSUTF8StringEncoding];
}

+ (NSDictionary *)queryForServerID:(NSString *)serverID keyClass:(CFTypeRef)keyClass {
    return @{(__bridge id)kSecClass: (__bridge id)kSecClassKey,
             (__bridge id)kSecAttrKeyType: (__bridge id)kSecAttrKeyTypeRSA,
             (__bridge id)kSecAttrKeyClass: (__bridge id)keyClass,
             (__bridge id)kSecAttrApplicationTag: [self tagData:serverID]};
}

+ (NSData *)publicKeyDataForServerID:(NSString *)serverID error:(NSError **)error {
    NSMutableDictionary *query = [[self queryForServerID:serverID keyClass:kSecAttrKeyClassPublic] mutableCopy];
    [query setObject:@YES forKey:(__bridge id)kSecReturnData];
    CFTypeRef copied = NULL;
    OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &copied);
    if (status != errSecSuccess) {
        if (error) *error = RBKeyError(status, @"Could not read this device's Surf key");
        return nil;
    }
    return CFBridgingRelease(copied);
}

+ (NSString *)ensurePublicKeyForServerID:(NSString *)serverID error:(NSError **)error {
    NSData *publicData = [self publicKeyDataForServerID:serverID error:nil];
    if (!publicData) {
        NSData *tag = [self tagData:serverID];
        NSDictionary *publicAttrs = @{(__bridge id)kSecAttrIsPermanent: @YES,
                                      (__bridge id)kSecAttrApplicationTag: tag};
        NSDictionary *privateAttrs = @{(__bridge id)kSecAttrIsPermanent: @YES,
                                       (__bridge id)kSecAttrApplicationTag: tag,
                                       (__bridge id)kSecAttrAccessible: (__bridge id)kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly};
        NSDictionary *parameters = @{(__bridge id)kSecAttrKeyType: (__bridge id)kSecAttrKeyTypeRSA,
                                     (__bridge id)kSecAttrKeySizeInBits: @2048,
                                     (__bridge id)kSecPublicKeyAttrs: publicAttrs,
                                     (__bridge id)kSecPrivateKeyAttrs: privateAttrs};
        SecKeyRef publicKey = NULL, privateKey = NULL;
        OSStatus status = SecKeyGeneratePair((__bridge CFDictionaryRef)parameters, &publicKey, &privateKey);
        if (publicKey) CFRelease(publicKey);
        if (privateKey) CFRelease(privateKey);
        if (status != errSecSuccess) {
            RBLogEvent(@"identity", @"error", @{@"status": @(status)}, @"Device key generation failed");
            if (error) *error = RBKeyError(status, @"Could not create this device's Surf key");
            return nil;
        }
        publicData = [self publicKeyDataForServerID:serverID error:error];
    }
    return publicData ? RBBase64URL(publicData) : nil;
}

+ (NSString *)deviceIDForServerID:(NSString *)serverID error:(NSError **)error {
    NSData *publicData = [self publicKeyDataForServerID:serverID error:error];
    if (!publicData) return nil;
    unsigned char digest[CC_SHA256_DIGEST_LENGTH];
    CC_SHA256([publicData bytes], (CC_LONG)[publicData length], digest);
    NSMutableString *result = [NSMutableString stringWithCapacity:CC_SHA256_DIGEST_LENGTH * 2];
    for (NSUInteger i = 0; i < CC_SHA256_DIGEST_LENGTH; i++) [result appendFormat:@"%02x", digest[i]];
    return result;
}

+ (NSString *)pairingPhraseForServerID:(NSString *)serverID error:(NSError **)error {
    NSData *publicData = [self publicKeyDataForServerID:serverID error:error];
    if (!publicData) return nil;
    static const char domain[] = "SURF-PAIR-V1";
    CC_SHA256_CTX context;
    CC_SHA256_Init(&context);
    CC_SHA256_Update(&context, domain, (CC_LONG)(sizeof(domain) - 1));
    unsigned char zero = 0;
    CC_SHA256_Update(&context, &zero, 1);
    NSData *serverData = [serverID dataUsingEncoding:NSUTF8StringEncoding];
    CC_SHA256_Update(&context, [serverData bytes], (CC_LONG)[serverData length]);
    CC_SHA256_Update(&context, [publicData bytes], (CC_LONG)[publicData length]);
    unsigned char digest[CC_SHA256_DIGEST_LENGTH];
    CC_SHA256_Final(digest, &context);
    static NSString *const words[] = {
        @"amber", @"anchor", @"apple", @"april", @"arrow", @"atlas", @"bamboo", @"beacon",
        @"birch", @"bloom", @"blue", @"bridge", @"brook", @"canyon", @"cedar", @"cloud",
        @"coral", @"crane", @"dawn", @"delta", @"drift", @"eagle", @"ember", @"fern",
        @"field", @"finch", @"forest", @"frost", @"garden", @"glade", @"gold", @"harbor",
        @"hazel", @"heron", @"island", @"jade", @"lake", @"leaf", @"lilac", @"luna",
        @"maple", @"meadow", @"mist", @"moon", @"north", @"ocean", @"olive", @"orchid",
        @"pearl", @"pine", @"quartz", @"rain", @"reed", @"river", @"robin", @"sage",
        @"shore", @"silver", @"sky", @"stone", @"sun", @"tide", @"willow", @"wren"
    };
    unsigned char indexes[] = {
        digest[0] >> 2,
        ((digest[0] & 3) << 4) | (digest[1] >> 4),
        ((digest[1] & 15) << 2) | (digest[2] >> 6),
        digest[2] & 63,
        digest[3] >> 2,
        ((digest[3] & 3) << 4) | (digest[4] >> 4)
    };
    return [NSString stringWithFormat:@"%@ %@ %@ %@ %@ %@",
            words[indexes[0]], words[indexes[1]], words[indexes[2]],
            words[indexes[3]], words[indexes[4]], words[indexes[5]]];
}

+ (NSString *)signAuthenticationForServerID:(NSString *)serverID deviceID:(NSString *)deviceID challengeID:(NSString *)challengeID nonce:(NSString *)nonce error:(NSError **)error {
    NSMutableDictionary *query = [[self queryForServerID:serverID keyClass:kSecAttrKeyClassPrivate] mutableCopy];
    [query setObject:@YES forKey:(__bridge id)kSecReturnRef];
    CFTypeRef copied = NULL;
    OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &copied);
    if (status != errSecSuccess) {
        if (error) *error = RBKeyError(status, @"This server's device key is missing");
        return nil;
    }
    SecKeyRef privateKey = (SecKeyRef)copied;
    NSMutableData *message = [NSMutableData data];
    unsigned char zero = 0;
    NSArray *parts = @[@"SURF-AUTH-V1", serverID ?: @"", deviceID ?: @"", challengeID ?: @"", nonce ?: @""];
    for (NSUInteger i = 0; i < [parts count]; i++) {
        if (i) [message appendBytes:&zero length:1];
        [message appendData:[[parts objectAtIndex:i] dataUsingEncoding:NSUTF8StringEncoding]];
    }
    unsigned char digest[CC_SHA256_DIGEST_LENGTH];
    CC_SHA256([message bytes], (CC_LONG)[message length], digest);
    size_t signatureLength = SecKeyGetBlockSize(privateKey);
    NSMutableData *signature = [NSMutableData dataWithLength:signatureLength];
    status = SecKeyRawSign(privateKey, kSecPaddingPKCS1SHA256, digest, sizeof(digest), [signature mutableBytes], &signatureLength);
    CFRelease(privateKey);
    if (status != errSecSuccess) {
        if (error) *error = RBKeyError(status, @"Could not prove this device's identity");
        return nil;
    }
    [signature setLength:signatureLength];
    return RBBase64URL(signature);
}

+ (void)deleteKeyForServerID:(NSString *)serverID {
    SecItemDelete((__bridge CFDictionaryRef)[self queryForServerID:serverID keyClass:kSecAttrKeyClassPrivate]);
    SecItemDelete((__bridge CFDictionaryRef)[self queryForServerID:serverID keyClass:kSecAttrKeyClassPublic]);
}

@end

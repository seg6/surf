#include <CommonCrypto/CommonDigest.h>
#include <errno.h>
#include <fcntl.h>
#include <spawn.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/wait.h>
#include <unistd.h>

extern char **environ;

static const char *source_path = "/var/mobile/Library/Surf/update.deb";
static const char *safe_path = "/tmp/surf-update.deb";

static int copy_package(const char *source) {
    if (strcmp(source, source_path) != 0) return 10;
    int in = open(source, O_RDONLY | O_NOFOLLOW);
    if (in < 0) return 11;
    struct stat st;
    if (fstat(in, &st) != 0 || !S_ISREG(st.st_mode) || st.st_size <= 0 || st.st_size > 50 * 1024 * 1024) {
        close(in);
        return 12;
    }
    unlink(safe_path);
    int out = open(safe_path, O_CREAT | O_EXCL | O_WRONLY, 0600);
    if (out < 0) { close(in); return 13; }
    char buffer[65536];
    ssize_t count;
    int result = 0;
    while ((count = read(in, buffer, sizeof(buffer))) > 0) {
        char *cursor = buffer;
        while (count > 0) {
            ssize_t written = write(out, cursor, (size_t)count);
            if (written <= 0) { result = 14; break; }
            cursor += written;
            count -= written;
        }
        if (result) break;
    }
    if (count < 0) result = 15;
    fsync(out);
    close(out);
    close(in);
    return result;
}

static int verify_hash(const char *wanted) {
    if (strlen(wanted) != CC_SHA256_DIGEST_LENGTH * 2) return 20;
    int fd = open(safe_path, O_RDONLY);
    if (fd < 0) return 21;
    CC_SHA256_CTX context;
    CC_SHA256_Init(&context);
    unsigned char buffer[65536], digest[CC_SHA256_DIGEST_LENGTH];
    ssize_t count;
    while ((count = read(fd, buffer, sizeof(buffer))) > 0) CC_SHA256_Update(&context, buffer, (CC_LONG)count);
    close(fd);
    if (count < 0) return 22;
    CC_SHA256_Final(digest, &context);
    char actual[CC_SHA256_DIGEST_LENGTH * 2 + 1];
    for (int i = 0; i < CC_SHA256_DIGEST_LENGTH; i++) sprintf(actual + i * 2, "%02x", digest[i]);
    return strcasecmp(actual, wanted) == 0 ? 0 : 23;
}

static int verify_metadata(const char *version) {
    FILE *pipe = popen("/usr/bin/dpkg-deb -f /tmp/surf-update.deb", "r");
    if (!pipe) return 30;
    char package[128] = {0}, actual_version[128] = {0}, architecture[128] = {0};
    char line[512];
    while (fgets(line, sizeof(line), pipe)) {
        char *newline = strpbrk(line, "\r\n");
        if (newline) *newline = '\0';
        if (strncmp(line, "Package: ", 9) == 0) {
            snprintf(package, sizeof(package), "%s", line + 9);
        } else if (strncmp(line, "Version: ", 9) == 0) {
            snprintf(actual_version, sizeof(actual_version), "%s", line + 9);
        } else if (strncmp(line, "Architecture: ", 14) == 0) {
            snprintf(architecture, sizeof(architecture), "%s", line + 14);
        }
    }
    int status = pclose(pipe);
    if (!package[0] || !actual_version[0] || !architecture[0] || status != 0) return 31;
    if (strcmp(package, "space.seg6.surf") != 0) return 32;
    size_t version_length = strlen(version);
    if (version_length == 0 || strncmp(actual_version, version, version_length) != 0 ||
        (actual_version[version_length] != '\0' && actual_version[version_length] != '-')) return 33;
    if (strcmp(architecture, "iphoneos-arm") != 0) return 34;
    return 0;
}

static int run(const char *path, char *const argv[]) {
    pid_t pid = 0;
    int error = posix_spawn(&pid, path, NULL, NULL, argv, environ);
    if (error) return error;
    int status = 0;
    if (waitpid(pid, &status, 0) < 0 || !WIFEXITED(status)) return 100;
    return WEXITSTATUS(status);
}

int main(int argc, char **argv) {
    if (argc != 4) return 2;
    if (setuid(0) != 0 || geteuid() != 0) return 3;
    int result = copy_package(argv[1]);
    if (!result) result = verify_hash(argv[2]);
    if (!result) result = verify_metadata(argv[3]);
    if (!result) {
        char *dpkg[] = {"/usr/bin/dpkg", "-i", (char *)safe_path, NULL};
        result = run(dpkg[0], dpkg);
    }
    if (!result && access("/bin/su", X_OK) == 0 && access("/usr/bin/uicache", X_OK) == 0) {
        char *uicache[] = {"/bin/su", "mobile", "-c", "/usr/bin/uicache", NULL};
        (void)run(uicache[0], uicache);
    }
    unlink(safe_path);
    return result;
}

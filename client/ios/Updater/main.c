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
static const char *result_path = "/var/mobile/Library/Surf/update-result";
static const char *result_temp_path = "/var/mobile/Library/Surf/update-result.tmp";
static const char *install_log_path = "/var/mobile/Library/Surf/update-install.log";
static const char *installed_helper = "/usr/libexec/surf-update-v2";
static const char *runner_prefix = "/tmp/surf-update-v2.";

static void write_result(const char *stage, int result, int install_result, int recovery_result,
                         const char *version, const char *hash) {
    unlink(result_temp_path);
    int fd = open(result_temp_path, O_CREAT | O_EXCL | O_WRONLY | O_NOFOLLOW, 0644);
    if (fd < 0) return;
    char buffer[768];
    int length = snprintf(buffer, sizeof(buffer),
                          "schema=2\nstage=%s\nresult=%d\ninstall_result=%d\n"
                          "recovery_result=%d\nversion=%s\nsha256=%s\nlog=%s\n",
                          stage ? stage : "unknown", result, install_result, recovery_result,
                          version ? version : "", hash ? hash : "", install_log_path);
    if (length > 0 && (size_t)length < sizeof(buffer)) {
        ssize_t written = write(fd, buffer, (size_t)length);
        if (written == length && fsync(fd) == 0) {
            close(fd);
            chmod(result_temp_path, 0644);
            if (rename(result_temp_path, result_path) == 0) return;
            unlink(result_temp_path);
            return;
        }
    }
    close(fd);
    unlink(result_temp_path);
}

static int reexec_private_copy(char **argv) {
    if (strncmp(argv[0], runner_prefix, strlen(runner_prefix)) == 0) return 0;
    int in = open(installed_helper, O_RDONLY | O_NOFOLLOW);
    if (in < 0) return errno;
    char runner[128];
    snprintf(runner, sizeof(runner), "%s%ld", runner_prefix, (long)getpid());
    unlink(runner);
    int out = open(runner, O_CREAT | O_EXCL | O_WRONLY | O_NOFOLLOW, 0700);
    if (out < 0) {
        int result = errno;
        close(in);
        return result;
    }
    char buffer[65536];
    ssize_t count;
    int result = 0;
    while ((count = read(in, buffer, sizeof(buffer))) > 0) {
        char *cursor = buffer;
        while (count > 0) {
            ssize_t written = write(out, cursor, (size_t)count);
            if (written <= 0) {
                result = errno ? errno : EIO;
                break;
            }
            cursor += written;
            count -= written;
        }
        if (result) break;
    }
    if (count < 0 && !result) result = errno;
    if (!result && fsync(out) != 0) result = errno;
    if (!result && fchmod(out, 0700) != 0) result = errno;
    close(out);
    close(in);
    if (result) {
        unlink(runner);
        return result;
    }
    argv[0] = runner;
    execve(runner, argv, environ);
    result = errno;
    unlink(runner);
    return result;
}

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

static int run_logged(const char *path, char *const argv[], int truncate) {
    posix_spawn_file_actions_t actions;
    int error = posix_spawn_file_actions_init(&actions);
    if (error) return error;
    int flags = O_CREAT | O_WRONLY | O_NOFOLLOW | (truncate ? O_TRUNC : O_APPEND);
    error = posix_spawn_file_actions_addopen(&actions, STDOUT_FILENO, install_log_path, flags, 0644);
    if (!error) {
        error = posix_spawn_file_actions_adddup2(&actions, STDOUT_FILENO, STDERR_FILENO);
    }
    pid_t pid = 0;
    if (!error) error = posix_spawn(&pid, path, &actions, NULL, argv, environ);
    posix_spawn_file_actions_destroy(&actions);
    if (error) return error;
    int status = 0;
    if (waitpid(pid, &status, 0) < 0 || !WIFEXITED(status)) return 100;
    return WEXITSTATUS(status);
}

static int installed_bundle_matches(const char *version) {
    if (!version || !version[0] || strlen(version) > 64) return 0;
    struct stat executable;
    if (stat("/Applications/Surf.app/Surf", &executable) != 0 ||
        !S_ISREG(executable.st_mode) || !(executable.st_mode & 0111)) return 0;
    FILE *plist = fopen("/Applications/Surf.app/Info.plist", "r");
    if (!plist) return 0;
    char wanted[160];
    int wanted_length = snprintf(wanted, sizeof(wanted),
                                 "<key>CFBundleShortVersionString</key>\n  <string>%s</string>",
                                 version);
    if (wanted_length <= 0 || (size_t)wanted_length >= sizeof(wanted)) {
        fclose(plist);
        return 0;
    }
    char contents[8192 + 1];
    size_t length = fread(contents, 1, sizeof(contents) - 1, plist);
    fclose(plist);
    contents[length] = '\0';
    return strstr(contents, wanted) != NULL;
}

int main(int argc, char **argv) {
    if (argc != 4) return 2;
    unlink(result_path);
    if (setuid(0) != 0 || geteuid() != 0) {
        write_result("privilege", 3, -1, -1, argv[3], argv[2]);
        return 3;
    }
    int bootstrap_result = reexec_private_copy(argv);
    if (bootstrap_result) {
        write_result("bootstrap", bootstrap_result, -1, -1, argv[3], argv[2]);
        return bootstrap_result;
    }
    const char *stage = "copy";
    int result = copy_package(argv[1]);
    int install_result = -1;
    int recovery_result = -1;
    if (!result) {
        stage = "checksum";
        result = verify_hash(argv[2]);
    }
    if (!result) {
        stage = "metadata";
        result = verify_metadata(argv[3]);
    }
    if (!result) {
        stage = "install";
        char *dpkg[] = {"/usr/bin/dpkg", "-i", (char *)safe_path, NULL};
        install_result = run_logged(dpkg[0], dpkg, 1);
        result = install_result;
        if (install_result != 0 && installed_bundle_matches(argv[3])) {
            char *configure[] = {"/usr/bin/dpkg", "--configure", "space.seg6.surf", NULL};
            recovery_result = run_logged(configure[0], configure, 0);
            if (recovery_result == 0) result = 0;
        }
        if (result == 0 && !installed_bundle_matches(argv[3])) {
            stage = "verify-install";
            result = 40;
        }
    }
    if (!result && access("/bin/su", X_OK) == 0 && access("/usr/bin/uicache", X_OK) == 0) {
        char *uicache[] = {"/bin/su", "mobile", "-c", "/usr/bin/uicache", NULL};
        (void)run(uicache[0], uicache);
    }
    if (!result) stage = "complete";
    write_result(stage, result, install_result, recovery_result, argv[3], argv[2]);
    unlink(safe_path);
    if (strncmp(argv[0], runner_prefix, strlen(runner_prefix)) == 0) unlink(argv[0]);
    return result;
}

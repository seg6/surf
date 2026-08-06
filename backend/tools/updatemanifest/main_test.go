package main

import (
	"strings"
	"testing"

	"surf-backend/internal/updater"
)

func TestRequirePlatformsAcceptsReleaseArchiveSet(t *testing.T) {
	assets := make(map[string]updater.Asset, len(requiredArchives))
	for _, platform := range requiredArchives {
		assets[platform] = updater.Asset{}
	}
	if err := requirePlatforms("Surf archives", assets, requiredArchives); err != nil {
		t.Fatalf("requirePlatforms() error = %v", err)
	}
}

func TestRequirePlatformsReportsMissingPlatform(t *testing.T) {
	assets := map[string]updater.Asset{
		"linux-amd64":   {},
		"windows-amd64": {},
		"darwin-amd64":  {},
		"darwin-arm64":  {},
	}
	err := requirePlatforms("Surf archives", assets, requiredArchives)
	if err == nil || !strings.Contains(err.Error(), "linux-arm64") {
		t.Fatalf("requirePlatforms() error = %v, want missing linux-arm64", err)
	}
}

func TestRequirePlatformsRejectsUnexpectedPlatform(t *testing.T) {
	assets := make(map[string]updater.Asset, len(requiredPackages)+1)
	for _, platform := range requiredPackages {
		assets[platform] = updater.Asset{}
	}
	assets["linux-riscv64"] = updater.Asset{}
	err := requirePlatforms("Surf desktop packages", assets, requiredPackages)
	if err == nil || !strings.Contains(err.Error(), "want exactly 2") {
		t.Fatalf("requirePlatforms() error = %v, want exact-count failure", err)
	}
}

func TestDarwinArchiveDoesNotRequirePlatformSpecificInstaller(t *testing.T) {
	if !surfArchive.MatchString("surf-1.2.3-darwin-arm64.tar.gz") {
		t.Fatal("Darwin application archive was not recognized")
	}
	if surfPackage.MatchString("surf-1.2.3-darwin-arm64.dmg") {
		t.Fatal("Darwin DMG was recognized as a required installer")
	}
}

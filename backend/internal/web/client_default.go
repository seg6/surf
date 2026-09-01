//go:build !client_bundle

package web

func embeddedPackage() []byte         { return nil }
func embeddedPackageMetadata() []byte { return nil }

//go:build client_bundle

package web

import _ "embed"

//go:embed client/client.deb
var clientPackageData []byte

//go:embed client/client.json
var clientMetadataData []byte

func embeddedPackage() []byte         { return clientPackageData }
func embeddedPackageMetadata() []byte { return clientMetadataData }

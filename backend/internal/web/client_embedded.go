//go:build client_bundle

package web

import _ "embed"

//go:embed client/client.deb
var clientPackageData []byte

func embeddedPackage() []byte { return clientPackageData }

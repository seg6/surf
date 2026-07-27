//go:build client_bundle

package clientupdate

import _ "embed"

//go:embed bundle/client.deb
var clientPackage []byte

func embeddedPackage() []byte { return clientPackage }

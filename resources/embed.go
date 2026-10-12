// Package resources embeds static assets that are not part of the web GUI
// (e.g. the donate QR code) so the compiled binary stays self-contained.
package resources

import "embed"

// FS exposes the files shipped in the resources directory.
//
//go:embed qrCode.png
var FS embed.FS

// AppsIDList is the catalog of App Store apps that were removed from the store
// but can still be downloaded by their numeric App ID.
//
// It is compiled into the binary so single-file builds - the macOS releases are
// one bare executable - keep the removed-apps search working. An
// Apps_ID_List.txt next to the running program still takes precedence, so the
// catalog can be replaced without rebuilding.
//
//go:embed Apps_ID_List.txt
var AppsIDList string

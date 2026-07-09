/*
Package media owns the media and attachment lifecycle for the messenger.

	The package keeps media metadata, upload/download targets, and deletion
	semantics transport-agnostic so the future API layer can map requests into the
	domain without embedding HTTP or gRPC specifics. Download targets are not
	direct blob-store links; they are application-side access targets that can be
	redeemed by an authenticated transport boundary later.
*/
package media

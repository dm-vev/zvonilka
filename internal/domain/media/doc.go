/*
Package media owns the media and attachment lifecycle for the messenger.

The package keeps media metadata, upload/download targets, and deletion
semantics transport-agnostic so the future API layer can map requests into the
domain without embedding HTTP or gRPC specifics.
*/
package media

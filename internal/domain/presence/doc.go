/*
Package presence resolves user presence and last-seen state.

The package keeps explicit presence settings separate from the identity layer
while deriving last-seen from identity activity. That keeps the auth domain
focused on authentication and lets future user-facing APIs project presence
without duplicating session or device state.
*/
package presence

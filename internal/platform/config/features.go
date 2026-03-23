package config

// FeatureConfig groups opt-in product flags for capabilities that ship later.
type FeatureConfig struct {
	FederationEnabled        bool
	CallsEnabled             bool
	SearchEnabled            bool
	ScheduledMessagesEnabled bool
	TranslationEnabled       bool
}

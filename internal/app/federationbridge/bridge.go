package federationbridge

import (
	"context"
	"strings"
	"time"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	domainfederation "github.com/dm-vev/zvonilka/internal/domain/federation"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (a *api) PullBridgeFragments(
	ctx context.Context,
	req *federationv1.PullBridgeFragmentsRequest,
) (*federationv1.PullBridgeFragmentsResponse, error) {
	if err := a.requireBridgeAccess(ctx); err != nil {
		return nil, err
	}

	fragments, link, cursor, hasMore, leaseToken, err := a.federation.PullBridgeFragments(
		ctx,
		req.GetPeerServerName(),
		req.GetLinkName(),
		int(req.GetLimit()),
	)
	if err != nil {
		return nil, grpcError(err)
	}

	page := make([]*federationv1.BundleFragment, 0, len(fragments))
	for _, fragment := range fragments {
		page = append(page, fragmentProto(fragment))
	}

	return &federationv1.PullBridgeFragmentsResponse{
		Fragments:  page,
		HasMore:    hasMore,
		Cursor:     cursorProto(cursor),
		Link:       linkProto(link),
		LeaseToken: leaseToken,
	}, nil
}

func (a *api) SubmitBridgeFragments(
	ctx context.Context,
	req *federationv1.SubmitBridgeFragmentsRequest,
) (*federationv1.SubmitBridgeFragmentsResponse, error) {
	if err := a.requireBridgeAccess(ctx); err != nil {
		return nil, err
	}

	params := make([]domainfederation.SaveFragmentParams, 0, len(req.GetFragments()))
	for _, fragment := range req.GetFragments() {
		if fragment == nil {
			continue
		}

		params = append(params, domainfederation.SaveFragmentParams{
			BundleID:      strings.TrimSpace(fragment.GetBundleId()),
			DedupKey:      strings.TrimSpace(fragment.GetDedupKey()),
			CursorFrom:    fragment.GetCursorFrom(),
			CursorTo:      fragment.GetCursorTo(),
			EventCount:    int(fragment.GetEventCount()),
			PayloadType:   fragment.GetPayloadType(),
			Compression:   compressionKindFromProto(fragment.GetCompression()),
			IntegrityHash: strings.TrimSpace(fragment.GetIntegrityHash()),
			AuthTag:       strings.TrimSpace(fragment.GetAuthTag()),
			FragmentIndex: int(fragment.GetFragmentIndex()),
			FragmentCount: int(fragment.GetFragmentCount()),
			Payload:       append([]byte(nil), fragment.GetPayload()...),
			AvailableAt:   timestampValue(fragment.GetAvailableAt()),
		})
	}

	accepted, assembled, cursor, err := a.federation.SubmitBridgeFragments(
		ctx,
		req.GetPeerServerName(),
		req.GetLinkName(),
		params,
	)
	if err != nil {
		return nil, grpcError(err)
	}

	acceptedIDs := make([]string, 0, len(accepted))
	for _, fragment := range accepted {
		acceptedIDs = append(acceptedIDs, fragment.ID)
	}
	assembledIDs := make([]string, 0, len(assembled))
	for _, bundle := range assembled {
		assembledIDs = append(assembledIDs, bundle.ID)
	}

	return &federationv1.SubmitBridgeFragmentsResponse{
		AcceptedFragmentIds: acceptedIDs,
		AssembledBundleIds:  assembledIDs,
		Cursor:              cursorProto(cursor),
	}, nil
}

func (a *api) AcknowledgeBridgeFragments(
	ctx context.Context,
	req *federationv1.AcknowledgeBridgeFragmentsRequest,
) (*federationv1.AcknowledgeBridgeFragmentsResponse, error) {
	if err := a.requireBridgeAccess(ctx); err != nil {
		return nil, err
	}

	updated, cursor, err := a.federation.AcknowledgeBridgeFragments(
		ctx,
		req.GetPeerServerName(),
		req.GetLinkName(),
		req.GetFragmentIds(),
		req.GetLeaseToken(),
		time.Now().UTC(),
	)
	if err != nil {
		return nil, grpcError(err)
	}

	acknowledgedIDs := make([]string, 0, len(updated))
	for _, fragment := range updated {
		acknowledgedIDs = append(acknowledgedIDs, fragment.ID)
	}

	return &federationv1.AcknowledgeBridgeFragmentsResponse{
		AcknowledgedFragmentIds: acknowledgedIDs,
		Cursor:                  cursorProto(cursor),
	}, nil
}

func linkProto(link domainfederation.Link) *federationv1.Link {
	return &federationv1.Link{
		LinkId:                   link.ID,
		PeerId:                   link.PeerID,
		Name:                     link.Name,
		Endpoint:                 link.Endpoint,
		TransportKind:            transportKindProto(link.TransportKind),
		DeliveryClass:            deliveryClassProto(link.DeliveryClass),
		DiscoveryMode:            discoveryModeProto(link.DiscoveryMode),
		MediaPolicy:              mediaPolicyProto(link.MediaPolicy),
		State:                    linkStateProto(link.State),
		MaxBundleBytes:           uint32(maxInt(0, link.MaxBundleBytes)),
		MaxFragmentBytes:         uint32(maxInt(0, link.MaxFragmentBytes)),
		AllowedConversationKinds: conversationKindsProto(link.AllowedConversationKinds),
		CreatedAt:                timestamppb.New(link.CreatedAt),
		UpdatedAt:                timestamppb.New(link.UpdatedAt),
		LastHealthyAt:            timestampOrNil(link.LastHealthyAt),
		LastError:                link.LastError,
	}
}

func cursorProto(cursor domainfederation.ReplicationCursor) *federationv1.ReplicationCursor {
	return &federationv1.ReplicationCursor{
		PeerId:             cursor.PeerID,
		LinkId:             cursor.LinkID,
		LastReceivedCursor: cursor.LastReceivedCursor,
		LastInboundCursor:  cursor.LastInboundCursor,
		LastOutboundCursor: cursor.LastOutboundCursor,
		LastAckedCursor:    cursor.LastAckedCursor,
		UpdatedAt:          timestamppb.New(cursor.UpdatedAt),
	}
}

func fragmentProto(fragment domainfederation.BundleFragment) *federationv1.BundleFragment {
	return &federationv1.BundleFragment{
		FragmentId:     fragment.ID,
		PeerId:         fragment.PeerID,
		LinkId:         fragment.LinkID,
		BundleId:       fragment.BundleID,
		DedupKey:       fragment.DedupKey,
		Direction:      bundleDirectionProto(fragment.Direction),
		CursorFrom:     fragment.CursorFrom,
		CursorTo:       fragment.CursorTo,
		EventCount:     uint32(maxInt(0, fragment.EventCount)),
		PayloadType:    fragment.PayloadType,
		Compression:    compressionKindProto(fragment.Compression),
		FragmentIndex:  uint32(maxInt(0, fragment.FragmentIndex)),
		FragmentCount:  uint32(maxInt(0, fragment.FragmentCount)),
		Payload:        append([]byte(nil), fragment.Payload...),
		State:          fragmentStateProto(fragment.State),
		IntegrityHash:  fragment.IntegrityHash,
		AuthTag:        fragment.AuthTag,
		LeaseExpiresAt: timestampOrNil(fragment.LeaseExpiresAt),
		AttemptCount:   uint32(maxInt(0, fragment.AttemptCount)),
		CreatedAt:      timestamppb.New(fragment.CreatedAt),
		AvailableAt:    timestamppb.New(fragment.AvailableAt),
		AckedAt:        timestampOrNil(fragment.AckedAt),
	}
}

func conversationKindsProto(values []domainfederation.ConversationKind) []commonv1.ConversationKind {
	converted := make([]commonv1.ConversationKind, 0, len(values))
	for _, value := range values {
		switch value {
		case domainfederation.ConversationKindDirect:
			converted = append(converted, commonv1.ConversationKind_CONVERSATION_KIND_DIRECT)
		case domainfederation.ConversationKindGroup:
			converted = append(converted, commonv1.ConversationKind_CONVERSATION_KIND_GROUP)
		case domainfederation.ConversationKindChannel:
			converted = append(converted, commonv1.ConversationKind_CONVERSATION_KIND_CHANNEL)
		}
	}

	return converted
}

func transportKindProto(value domainfederation.TransportKind) federationv1.TransportKind {
	switch value {
	case domainfederation.TransportKindHTTPS:
		return federationv1.TransportKind_TRANSPORT_KIND_HTTPS
	case domainfederation.TransportKindMeshtastic:
		return federationv1.TransportKind_TRANSPORT_KIND_MESHTASTIC
	case domainfederation.TransportKindMeshCore:
		return federationv1.TransportKind_TRANSPORT_KIND_MESHCORE
	case domainfederation.TransportKindCustomDTN:
		return federationv1.TransportKind_TRANSPORT_KIND_CUSTOM_DTN
	default:
		return federationv1.TransportKind_TRANSPORT_KIND_UNSPECIFIED
	}
}

func deliveryClassProto(value domainfederation.DeliveryClass) federationv1.DeliveryClass {
	switch value {
	case domainfederation.DeliveryClassRealtime:
		return federationv1.DeliveryClass_DELIVERY_CLASS_REALTIME
	case domainfederation.DeliveryClassDelayTolerant:
		return federationv1.DeliveryClass_DELIVERY_CLASS_DELAY_TOLERANT
	case domainfederation.DeliveryClassUltraConstrained:
		return federationv1.DeliveryClass_DELIVERY_CLASS_ULTRA_CONSTRAINED
	default:
		return federationv1.DeliveryClass_DELIVERY_CLASS_UNSPECIFIED
	}
}

func discoveryModeProto(value domainfederation.DiscoveryMode) federationv1.DiscoveryMode {
	switch value {
	case domainfederation.DiscoveryModeManual:
		return federationv1.DiscoveryMode_DISCOVERY_MODE_MANUAL
	case domainfederation.DiscoveryModeDNS:
		return federationv1.DiscoveryMode_DISCOVERY_MODE_DNS
	case domainfederation.DiscoveryModeWellKnown:
		return federationv1.DiscoveryMode_DISCOVERY_MODE_WELL_KNOWN
	case domainfederation.DiscoveryModeBridgeAnnounced:
		return federationv1.DiscoveryMode_DISCOVERY_MODE_BRIDGE_ANNOUNCED
	default:
		return federationv1.DiscoveryMode_DISCOVERY_MODE_UNSPECIFIED
	}
}

func mediaPolicyProto(value domainfederation.MediaPolicy) federationv1.MediaPolicy {
	switch value {
	case domainfederation.MediaPolicyReferenceProxy:
		return federationv1.MediaPolicy_MEDIA_POLICY_REFERENCE_PROXY
	case domainfederation.MediaPolicyBackgroundReplication:
		return federationv1.MediaPolicy_MEDIA_POLICY_BACKGROUND_REPLICATION
	case domainfederation.MediaPolicyDisabled:
		return federationv1.MediaPolicy_MEDIA_POLICY_DISABLED
	default:
		return federationv1.MediaPolicy_MEDIA_POLICY_UNSPECIFIED
	}
}

func linkStateProto(value domainfederation.LinkState) federationv1.LinkState {
	switch value {
	case domainfederation.LinkStateActive:
		return federationv1.LinkState_LINK_STATE_ACTIVE
	case domainfederation.LinkStatePaused:
		return federationv1.LinkState_LINK_STATE_PAUSED
	case domainfederation.LinkStateDegraded:
		return federationv1.LinkState_LINK_STATE_DEGRADED
	case domainfederation.LinkStateDeleted:
		return federationv1.LinkState_LINK_STATE_DELETED
	default:
		return federationv1.LinkState_LINK_STATE_UNSPECIFIED
	}
}

func bundleDirectionProto(value domainfederation.BundleDirection) federationv1.BundleDirection {
	switch value {
	case domainfederation.BundleDirectionInbound:
		return federationv1.BundleDirection_BUNDLE_DIRECTION_INBOUND
	case domainfederation.BundleDirectionOutbound:
		return federationv1.BundleDirection_BUNDLE_DIRECTION_OUTBOUND
	default:
		return federationv1.BundleDirection_BUNDLE_DIRECTION_UNSPECIFIED
	}
}

func fragmentStateProto(value domainfederation.FragmentState) federationv1.FragmentState {
	switch value {
	case domainfederation.FragmentStateQueued:
		return federationv1.FragmentState_FRAGMENT_STATE_QUEUED
	case domainfederation.FragmentStateClaimed:
		return federationv1.FragmentState_FRAGMENT_STATE_CLAIMED
	case domainfederation.FragmentStateAccepted:
		return federationv1.FragmentState_FRAGMENT_STATE_ACCEPTED
	case domainfederation.FragmentStateAcknowledged:
		return federationv1.FragmentState_FRAGMENT_STATE_ACKNOWLEDGED
	case domainfederation.FragmentStateAssembled:
		return federationv1.FragmentState_FRAGMENT_STATE_ASSEMBLED
	case domainfederation.FragmentStateFailed:
		return federationv1.FragmentState_FRAGMENT_STATE_FAILED
	default:
		return federationv1.FragmentState_FRAGMENT_STATE_UNSPECIFIED
	}
}

func compressionKindFromProto(value federationv1.CompressionKind) domainfederation.CompressionKind {
	switch value {
	case federationv1.CompressionKind_COMPRESSION_KIND_GZIP:
		return domainfederation.CompressionKindGzip
	case federationv1.CompressionKind_COMPRESSION_KIND_NONE:
		return domainfederation.CompressionKindNone
	default:
		return domainfederation.CompressionKindUnspecified
	}
}

func compressionKindProto(value domainfederation.CompressionKind) federationv1.CompressionKind {
	switch value {
	case domainfederation.CompressionKindGzip:
		return federationv1.CompressionKind_COMPRESSION_KIND_GZIP
	case domainfederation.CompressionKindNone:
		return federationv1.CompressionKind_COMPRESSION_KIND_NONE
	default:
		return federationv1.CompressionKind_COMPRESSION_KIND_UNSPECIFIED
	}
}

func timestampOrNil(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}

	return timestamppb.New(value.UTC())
}

func timestampValue(value *timestamppb.Timestamp) time.Time {
	if value == nil {
		return time.Time{}
	}

	return value.AsTime().UTC()
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

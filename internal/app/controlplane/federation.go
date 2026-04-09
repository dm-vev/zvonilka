package controlplane

import (
	"context"
	"strings"
	"time"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	domainfederation "github.com/dm-vev/zvonilka/internal/domain/federation"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	federationPeersPagePrefix = "federation_peers"
	federationLinksPagePrefix = "federation_links"
)

// CreatePeer persists one federation peer and returns its onboarding secret.
func (a *api) CreatePeer(
	ctx context.Context,
	req *federationv1.CreatePeerRequest,
) (*federationv1.CreatePeerResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}
	if _, err := a.requireRoles(ctx, domainidentity.RoleOwner, domainidentity.RoleAdmin); err != nil {
		return nil, err
	}

	peer, sharedSecret, generated, err := service.CreatePeer(ctx, domainfederation.CreatePeerParams{
		ServerName:   req.GetServerName(),
		BaseURL:      req.GetBaseUrl(),
		Capabilities: capabilitiesFromProto(req.GetCapabilities()),
		Trusted:      req.GetTrusted(),
		SharedSecret: req.GetSharedSecret(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &federationv1.CreatePeerResponse{
		Peer:                  peerProto(peer),
		SharedSecret:          sharedSecret,
		GeneratedSharedSecret: generated,
	}, nil
}

// GetPeer resolves one peer by ID or server name.
func (a *api) GetPeer(
	ctx context.Context,
	req *federationv1.GetPeerRequest,
) (*federationv1.GetPeerResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}
	if _, err := a.requireRoles(
		ctx,
		domainidentity.RoleOwner,
		domainidentity.RoleAdmin,
		domainidentity.RoleSupport,
		domainidentity.RoleAuditor,
	); err != nil {
		return nil, err
	}

	var peer domainfederation.Peer
	switch lookup := req.GetLookup().(type) {
	case *federationv1.GetPeerRequest_PeerId:
		peer, err = service.PeerByID(ctx, lookup.PeerId)
	case *federationv1.GetPeerRequest_ServerName:
		peer, err = service.PeerByServerName(ctx, lookup.ServerName)
	default:
		return nil, status.Error(codes.InvalidArgument, "peer lookup is required")
	}
	if err != nil {
		return nil, grpcError(err)
	}

	return &federationv1.GetPeerResponse{Peer: peerProto(peer)}, nil
}

// ListPeers lists peers visible to the current admin actor.
func (a *api) ListPeers(
	ctx context.Context,
	req *federationv1.ListPeersRequest,
) (*federationv1.ListPeersResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}
	if _, err := a.requireRoles(
		ctx,
		domainidentity.RoleOwner,
		domainidentity.RoleAdmin,
		domainidentity.RoleSupport,
		domainidentity.RoleAuditor,
	); err != nil {
		return nil, err
	}

	peers, err := service.ListPeers(ctx, peerStateFromProto(req.GetStateFilter()))
	if err != nil {
		return nil, grpcError(err)
	}

	offset, err := decodeOffset(req.GetPage(), federationPeersPagePrefix)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(peers) {
		end = len(peers)
	}

	page := make([]*federationv1.Peer, 0, max(0, end-offset))
	if offset < len(peers) {
		for _, peer := range peers[offset:end] {
			page = append(page, peerProto(peer))
		}
	}

	nextToken := ""
	if end < len(peers) {
		nextToken = offsetToken(federationPeersPagePrefix, end)
	}

	return &federationv1.ListPeersResponse{
		Peers: page,
		Page: &commonv1.PageResponse{
			NextPageToken: nextToken,
			TotalSize:     uint64(len(peers)),
		},
	}, nil
}

// UpdatePeer persists mutable peer fields.
func (a *api) UpdatePeer(
	ctx context.Context,
	req *federationv1.UpdatePeerRequest,
) (*federationv1.UpdatePeerResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}
	if _, err := a.requireRoles(ctx, domainidentity.RoleOwner, domainidentity.RoleAdmin); err != nil {
		return nil, err
	}
	if req.GetPeer() == nil || strings.TrimSpace(req.GetPeer().GetPeerId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "peer_id is required")
	}

	params := domainfederation.UpdatePeerParams{
		PeerID: req.GetPeer().GetPeerId(),
	}
	paths := req.GetUpdateMask().GetPaths()
	if len(paths) == 0 {
		paths = []string{"server_name", "base_url", "capabilities", "trusted", "state"}
	}
	for _, path := range paths {
		switch path {
		case "server_name":
			value := req.GetPeer().GetServerName()
			params.ServerName = &value
		case "base_url":
			value := req.GetPeer().GetBaseUrl()
			params.BaseURL = &value
		case "capabilities":
			value := capabilitiesFromProto(req.GetPeer().GetCapabilities())
			params.Capabilities = &value
		case "trusted":
			value := req.GetPeer().GetTrusted()
			params.Trusted = &value
		case "state":
			value := peerStateFromProto(req.GetPeer().GetState())
			params.State = &value
		default:
			return nil, status.Errorf(codes.InvalidArgument, "unsupported peer update field %q", path)
		}
	}

	peer, err := service.UpdatePeer(ctx, params)
	if err != nil {
		return nil, grpcError(err)
	}

	return &federationv1.UpdatePeerResponse{Peer: peerProto(peer)}, nil
}

// CreateLink persists one federation link for a peer.
func (a *api) CreateLink(
	ctx context.Context,
	req *federationv1.CreateLinkRequest,
) (*federationv1.CreateLinkResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}
	if _, err := a.requireRoles(ctx, domainidentity.RoleOwner, domainidentity.RoleAdmin); err != nil {
		return nil, err
	}

	link, err := service.CreateLink(ctx, domainfederation.CreateLinkParams{
		PeerID:                   req.GetPeerId(),
		Name:                     req.GetName(),
		Endpoint:                 req.GetEndpoint(),
		TransportKind:            transportKindFromProto(req.GetTransportKind()),
		DeliveryClass:            deliveryClassFromProto(req.GetDeliveryClass()),
		DiscoveryMode:            discoveryModeFromProto(req.GetDiscoveryMode()),
		MediaPolicy:              mediaPolicyFromProto(req.GetMediaPolicy()),
		MaxBundleBytes:           int(req.GetMaxBundleBytes()),
		MaxFragmentBytes:         int(req.GetMaxFragmentBytes()),
		AllowedConversationKinds: conversationKindsFromProto(req.GetAllowedConversationKinds()),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &federationv1.CreateLinkResponse{Link: linkProto(link)}, nil
}

// GetLink resolves one link by ID.
func (a *api) GetLink(
	ctx context.Context,
	req *federationv1.GetLinkRequest,
) (*federationv1.GetLinkResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}
	if _, err := a.requireRoles(
		ctx,
		domainidentity.RoleOwner,
		domainidentity.RoleAdmin,
		domainidentity.RoleSupport,
		domainidentity.RoleAuditor,
	); err != nil {
		return nil, err
	}

	link, err := service.LinkByID(ctx, req.GetLinkId())
	if err != nil {
		return nil, grpcError(err)
	}

	return &federationv1.GetLinkResponse{Link: linkProto(link)}, nil
}

// ListLinks lists links visible to the current admin actor.
func (a *api) ListLinks(
	ctx context.Context,
	req *federationv1.ListLinksRequest,
) (*federationv1.ListLinksResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}
	if _, err := a.requireRoles(
		ctx,
		domainidentity.RoleOwner,
		domainidentity.RoleAdmin,
		domainidentity.RoleSupport,
		domainidentity.RoleAuditor,
	); err != nil {
		return nil, err
	}

	links, err := service.ListLinks(ctx, req.GetPeerId(), linkStateFromProto(req.GetStateFilter()))
	if err != nil {
		return nil, grpcError(err)
	}

	offset, err := decodeOffset(req.GetPage(), federationLinksPagePrefix)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(links) {
		end = len(links)
	}

	page := make([]*federationv1.Link, 0, max(0, end-offset))
	if offset < len(links) {
		for _, link := range links[offset:end] {
			page = append(page, linkProto(link))
		}
	}

	nextToken := ""
	if end < len(links) {
		nextToken = offsetToken(federationLinksPagePrefix, end)
	}

	return &federationv1.ListLinksResponse{
		Links: page,
		Page: &commonv1.PageResponse{
			NextPageToken: nextToken,
			TotalSize:     uint64(len(links)),
		},
	}, nil
}

// UpdateLink persists mutable link fields.
func (a *api) UpdateLink(
	ctx context.Context,
	req *federationv1.UpdateLinkRequest,
) (*federationv1.UpdateLinkResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}
	if _, err := a.requireRoles(ctx, domainidentity.RoleOwner, domainidentity.RoleAdmin); err != nil {
		return nil, err
	}
	if req.GetLink() == nil || strings.TrimSpace(req.GetLink().GetLinkId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "link_id is required")
	}

	params := domainfederation.UpdateLinkParams{
		LinkID: req.GetLink().GetLinkId(),
	}
	paths := req.GetUpdateMask().GetPaths()
	if len(paths) == 0 {
		paths = []string{
			"name",
			"endpoint",
			"transport_kind",
			"delivery_class",
			"discovery_mode",
			"media_policy",
			"state",
			"max_bundle_bytes",
			"max_fragment_bytes",
			"allowed_conversation_kinds",
			"last_error",
		}
	}
	for _, path := range paths {
		switch path {
		case "name":
			value := req.GetLink().GetName()
			params.Name = &value
		case "endpoint":
			value := req.GetLink().GetEndpoint()
			params.Endpoint = &value
		case "transport_kind":
			value := transportKindFromProto(req.GetLink().GetTransportKind())
			params.TransportKind = &value
		case "delivery_class":
			value := deliveryClassFromProto(req.GetLink().GetDeliveryClass())
			params.DeliveryClass = &value
		case "discovery_mode":
			value := discoveryModeFromProto(req.GetLink().GetDiscoveryMode())
			params.DiscoveryMode = &value
		case "media_policy":
			value := mediaPolicyFromProto(req.GetLink().GetMediaPolicy())
			params.MediaPolicy = &value
		case "state":
			value := linkStateFromProto(req.GetLink().GetState())
			params.State = &value
		case "max_bundle_bytes":
			value := int(req.GetLink().GetMaxBundleBytes())
			params.MaxBundleBytes = &value
		case "max_fragment_bytes":
			value := int(req.GetLink().GetMaxFragmentBytes())
			params.MaxFragmentBytes = &value
		case "allowed_conversation_kinds":
			value := conversationKindsFromProto(req.GetLink().GetAllowedConversationKinds())
			params.AllowedConversationKinds = &value
		case "last_error":
			value := req.GetLink().GetLastError()
			params.LastError = &value
		default:
			return nil, status.Errorf(codes.InvalidArgument, "unsupported link update field %q", path)
		}
	}

	link, err := service.UpdateLink(ctx, params)
	if err != nil {
		return nil, grpcError(err)
	}

	return &federationv1.UpdateLinkResponse{Link: linkProto(link)}, nil
}

// PauseLink transitions a link to paused.
func (a *api) PauseLink(
	ctx context.Context,
	req *federationv1.PauseLinkRequest,
) (*federationv1.PauseLinkResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}
	if _, err := a.requireRoles(ctx, domainidentity.RoleOwner, domainidentity.RoleAdmin); err != nil {
		return nil, err
	}

	link, err := service.PauseLink(ctx, req.GetLinkId())
	if err != nil {
		return nil, grpcError(err)
	}

	return &federationv1.PauseLinkResponse{Link: linkProto(link)}, nil
}

// ResumeLink transitions a paused link back to active.
func (a *api) ResumeLink(
	ctx context.Context,
	req *federationv1.ResumeLinkRequest,
) (*federationv1.ResumeLinkResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}
	if _, err := a.requireRoles(ctx, domainidentity.RoleOwner, domainidentity.RoleAdmin); err != nil {
		return nil, err
	}

	link, err := service.ResumeLink(ctx, req.GetLinkId())
	if err != nil {
		return nil, grpcError(err)
	}

	return &federationv1.ResumeLinkResponse{Link: linkProto(link)}, nil
}

// DeleteLink soft-deletes a link.
func (a *api) DeleteLink(
	ctx context.Context,
	req *federationv1.DeleteLinkRequest,
) (*federationv1.DeleteLinkResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}
	if _, err := a.requireRoles(ctx, domainidentity.RoleOwner, domainidentity.RoleAdmin); err != nil {
		return nil, err
	}

	link, err := service.DeleteLink(ctx, req.GetLinkId())
	if err != nil {
		return nil, grpcError(err)
	}

	return &federationv1.DeleteLinkResponse{Link: linkProto(link)}, nil
}

// GetReplicationCursor returns the durable replication cursor for one link.
func (a *api) GetReplicationCursor(
	ctx context.Context,
	req *federationv1.GetReplicationCursorRequest,
) (*federationv1.GetReplicationCursorResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}
	if _, err := a.requireRoles(
		ctx,
		domainidentity.RoleOwner,
		domainidentity.RoleAdmin,
		domainidentity.RoleSupport,
		domainidentity.RoleAuditor,
	); err != nil {
		return nil, err
	}

	cursor, err := service.ReplicationCursorByPeerAndLink(ctx, req.GetPeerId(), req.GetLinkId())
	if err != nil {
		return nil, grpcError(err)
	}

	return &federationv1.GetReplicationCursorResponse{Cursor: cursorProto(cursor)}, nil
}

// PushBundles accepts inbound replication bundles from an authenticated federation peer.
func (a *api) PushBundles(
	ctx context.Context,
	req *federationv1.PushBundlesRequest,
) (*federationv1.PushBundlesResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}

	peer, err := a.requireFederationPeer(ctx)
	if err != nil {
		return nil, err
	}
	if requestServer := strings.TrimSpace(strings.ToLower(req.GetServerName())); requestServer != "" && requestServer != peer.ServerName {
		return nil, status.Error(codes.PermissionDenied, "server_name does not match federation credentials")
	}
	linkName := strings.TrimSpace(strings.ToLower(req.GetLinkName()))
	if linkName == "" {
		return nil, status.Error(codes.InvalidArgument, "link_name is required")
	}

	link, err := service.LinkByPeerAndName(ctx, peer.ID, linkName)
	if err != nil {
		return nil, grpcError(err)
	}

	params := make([]domainfederation.SaveBundleParams, 0, len(req.GetBundles()))
	for _, bundle := range req.GetBundles() {
		if bundle == nil {
			continue
		}

		params = append(params, domainfederation.SaveBundleParams{
			PeerID:      peer.ID,
			LinkID:      link.ID,
			DedupKey:    bundle.GetDedupKey(),
			CursorFrom:  bundle.GetCursorFrom(),
			CursorTo:    bundle.GetCursorTo(),
			EventCount:  int(bundle.GetEventCount()),
			PayloadType: bundle.GetPayloadType(),
			Payload:     append([]byte(nil), bundle.GetPayload()...),
			Compression: compressionKindFromProto(bundle.GetCompression()),
			AvailableAt: timestampValue(bundle.GetAvailableAt()),
			ExpiresAt:   timestampValue(bundle.GetExpiresAt()),
		})
	}

	accepted, cursor, err := service.PushInboundBundles(ctx, peer.ID, link.ID, params)
	if err != nil {
		return nil, grpcError(err)
	}

	acceptedIDs := make([]string, 0, len(accepted))
	for _, bundle := range accepted {
		acceptedIDs = append(acceptedIDs, bundle.ID)
	}

	return &federationv1.PushBundlesResponse{
		AcceptedBundleIds: acceptedIDs,
		Cursor:            cursorProto(cursor),
	}, nil
}

// PullBundles returns outbound replication bundles for an authenticated federation peer.
func (a *api) PullBundles(
	ctx context.Context,
	req *federationv1.PullBundlesRequest,
) (*federationv1.PullBundlesResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}

	peer, err := a.requireFederationPeer(ctx)
	if err != nil {
		return nil, err
	}
	if requestServer := strings.TrimSpace(strings.ToLower(req.GetServerName())); requestServer != "" && requestServer != peer.ServerName {
		return nil, status.Error(codes.PermissionDenied, "server_name does not match federation credentials")
	}
	linkName := strings.TrimSpace(strings.ToLower(req.GetLinkName()))
	if linkName == "" {
		return nil, status.Error(codes.InvalidArgument, "link_name is required")
	}

	link, err := service.LinkByPeerAndName(ctx, peer.ID, linkName)
	if err != nil {
		return nil, grpcError(err)
	}

	bundles, cursor, hasMore, err := service.PullOutboundBundles(
		ctx,
		peer.ID,
		link.ID,
		req.GetAfterCursor(),
		int(req.GetLimit()),
	)
	if err != nil {
		return nil, grpcError(err)
	}

	nextCursor := req.GetAfterCursor()
	if len(bundles) > 0 {
		nextCursor = bundles[len(bundles)-1].CursorTo
	}

	page := make([]*federationv1.Bundle, 0, len(bundles))
	for _, bundle := range bundles {
		page = append(page, bundleProto(bundle))
	}

	return &federationv1.PullBundlesResponse{
		Bundles:    page,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Cursor:     cursorProto(cursor),
	}, nil
}

// AcknowledgeBundles advances the durable acknowledgement cursor for an authenticated federation peer.
func (a *api) AcknowledgeBundles(
	ctx context.Context,
	req *federationv1.AcknowledgeBundlesRequest,
) (*federationv1.AcknowledgeBundlesResponse, error) {
	service, err := a.requireFederationService()
	if err != nil {
		return nil, err
	}

	peer, err := a.requireFederationPeer(ctx)
	if err != nil {
		return nil, err
	}
	if requestServer := strings.TrimSpace(strings.ToLower(req.GetServerName())); requestServer != "" && requestServer != peer.ServerName {
		return nil, status.Error(codes.PermissionDenied, "server_name does not match federation credentials")
	}
	linkName := strings.TrimSpace(strings.ToLower(req.GetLinkName()))
	if linkName == "" {
		return nil, status.Error(codes.InvalidArgument, "link_name is required")
	}

	link, err := service.LinkByPeerAndName(ctx, peer.ID, linkName)
	if err != nil {
		return nil, grpcError(err)
	}

	cursor, err := service.AcknowledgeBundles(ctx, domainfederation.AcknowledgeBundlesParams{
		PeerID:         peer.ID,
		LinkID:         link.ID,
		UpToCursor:     req.GetUpToCursor(),
		BundleIDs:      req.GetBundleIds(),
		AcknowledgedAt: time.Now().UTC(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &federationv1.AcknowledgeBundlesResponse{
		AcknowledgedBundleIds: append([]string(nil), req.GetBundleIds()...),
		Cursor:                cursorProto(cursor),
	}, nil
}

func (a *api) requireFederationService() (*domainfederation.Service, error) {
	if a == nil || a.federation == nil {
		return nil, status.Error(codes.Unimplemented, "federation service unavailable")
	}

	return a.federation, nil
}

func peerProto(peer domainfederation.Peer) *federationv1.Peer {
	return &federationv1.Peer{
		PeerId:                  peer.ID,
		ServerName:              peer.ServerName,
		BaseUrl:                 peer.BaseURL,
		Capabilities:            capabilitiesProto(peer.Capabilities),
		Trusted:                 peer.Trusted,
		State:                   peerStateProto(peer.State),
		CreatedAt:               timestamppb.New(peer.CreatedAt),
		UpdatedAt:               timestamppb.New(peer.UpdatedAt),
		LastSeenAt:              timestampOrNil(peer.LastSeenAt),
		VerificationFingerprint: peer.VerificationFingerprint,
	}
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

func bundleProto(bundle domainfederation.Bundle) *federationv1.Bundle {
	return &federationv1.Bundle{
		BundleId:    bundle.ID,
		PeerId:      bundle.PeerID,
		LinkId:      bundle.LinkID,
		DedupKey:    bundle.DedupKey,
		Direction:   bundleDirectionProto(bundle.Direction),
		CursorFrom:  bundle.CursorFrom,
		CursorTo:    bundle.CursorTo,
		EventCount:  uint32(maxInt(0, bundle.EventCount)),
		PayloadType: bundle.PayloadType,
		Payload:     append([]byte(nil), bundle.Payload...),
		Compression: compressionKindProto(bundle.Compression),
		State:       bundleStateProto(bundle.State),
		CreatedAt:   timestamppb.New(bundle.CreatedAt),
		AvailableAt: timestamppb.New(bundle.AvailableAt),
		ExpiresAt:   timestampOrNil(bundle.ExpiresAt),
		AckedAt:     timestampOrNil(bundle.AckedAt),
	}
}

func capabilitiesFromProto(values []commonv1.FederationCapability) []domainfederation.Capability {
	if len(values) == 0 {
		return nil
	}

	converted := make([]domainfederation.Capability, 0, len(values))
	for _, value := range values {
		switch value {
		case commonv1.FederationCapability_FEDERATION_CAPABILITY_EVENT_REPLICATION:
			converted = append(converted, domainfederation.CapabilityEventReplication)
		case commonv1.FederationCapability_FEDERATION_CAPABILITY_DIRECTORY_SYNC:
			converted = append(converted, domainfederation.CapabilityDirectorySync)
		case commonv1.FederationCapability_FEDERATION_CAPABILITY_MEDIA_PROXY:
			converted = append(converted, domainfederation.CapabilityMediaProxy)
		case commonv1.FederationCapability_FEDERATION_CAPABILITY_BOT_BRIDGE:
			converted = append(converted, domainfederation.CapabilityBotBridge)
		case commonv1.FederationCapability_FEDERATION_CAPABILITY_SEARCH_MIRROR:
			converted = append(converted, domainfederation.CapabilitySearchMirror)
		}
	}

	return converted
}

func capabilitiesProto(values []domainfederation.Capability) []commonv1.FederationCapability {
	converted := make([]commonv1.FederationCapability, 0, len(values))
	for _, value := range values {
		switch value {
		case domainfederation.CapabilityEventReplication:
			converted = append(converted, commonv1.FederationCapability_FEDERATION_CAPABILITY_EVENT_REPLICATION)
		case domainfederation.CapabilityDirectorySync:
			converted = append(converted, commonv1.FederationCapability_FEDERATION_CAPABILITY_DIRECTORY_SYNC)
		case domainfederation.CapabilityMediaProxy:
			converted = append(converted, commonv1.FederationCapability_FEDERATION_CAPABILITY_MEDIA_PROXY)
		case domainfederation.CapabilityBotBridge:
			converted = append(converted, commonv1.FederationCapability_FEDERATION_CAPABILITY_BOT_BRIDGE)
		case domainfederation.CapabilitySearchMirror:
			converted = append(converted, commonv1.FederationCapability_FEDERATION_CAPABILITY_SEARCH_MIRROR)
		}
	}

	return converted
}

func peerStateFromProto(value federationv1.PeerState) domainfederation.PeerState {
	switch value {
	case federationv1.PeerState_PEER_STATE_PENDING:
		return domainfederation.PeerStatePending
	case federationv1.PeerState_PEER_STATE_ACTIVE:
		return domainfederation.PeerStateActive
	case federationv1.PeerState_PEER_STATE_DEGRADED:
		return domainfederation.PeerStateDegraded
	case federationv1.PeerState_PEER_STATE_REVOKED:
		return domainfederation.PeerStateRevoked
	default:
		return domainfederation.PeerStateUnspecified
	}
}

func peerStateProto(value domainfederation.PeerState) federationv1.PeerState {
	switch value {
	case domainfederation.PeerStatePending:
		return federationv1.PeerState_PEER_STATE_PENDING
	case domainfederation.PeerStateActive:
		return federationv1.PeerState_PEER_STATE_ACTIVE
	case domainfederation.PeerStateDegraded:
		return federationv1.PeerState_PEER_STATE_DEGRADED
	case domainfederation.PeerStateRevoked:
		return federationv1.PeerState_PEER_STATE_REVOKED
	default:
		return federationv1.PeerState_PEER_STATE_UNSPECIFIED
	}
}

func transportKindFromProto(value federationv1.TransportKind) domainfederation.TransportKind {
	switch value {
	case federationv1.TransportKind_TRANSPORT_KIND_HTTPS:
		return domainfederation.TransportKindHTTPS
	case federationv1.TransportKind_TRANSPORT_KIND_MESHTASTIC:
		return domainfederation.TransportKindMeshtastic
	case federationv1.TransportKind_TRANSPORT_KIND_MESHCORE:
		return domainfederation.TransportKindMeshCore
	case federationv1.TransportKind_TRANSPORT_KIND_CUSTOM_DTN:
		return domainfederation.TransportKindCustomDTN
	default:
		return domainfederation.TransportKindUnspecified
	}
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

func deliveryClassFromProto(value federationv1.DeliveryClass) domainfederation.DeliveryClass {
	switch value {
	case federationv1.DeliveryClass_DELIVERY_CLASS_REALTIME:
		return domainfederation.DeliveryClassRealtime
	case federationv1.DeliveryClass_DELIVERY_CLASS_DELAY_TOLERANT:
		return domainfederation.DeliveryClassDelayTolerant
	case federationv1.DeliveryClass_DELIVERY_CLASS_ULTRA_CONSTRAINED:
		return domainfederation.DeliveryClassUltraConstrained
	default:
		return domainfederation.DeliveryClassUnspecified
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

func discoveryModeFromProto(value federationv1.DiscoveryMode) domainfederation.DiscoveryMode {
	switch value {
	case federationv1.DiscoveryMode_DISCOVERY_MODE_MANUAL:
		return domainfederation.DiscoveryModeManual
	case federationv1.DiscoveryMode_DISCOVERY_MODE_DNS:
		return domainfederation.DiscoveryModeDNS
	case federationv1.DiscoveryMode_DISCOVERY_MODE_WELL_KNOWN:
		return domainfederation.DiscoveryModeWellKnown
	case federationv1.DiscoveryMode_DISCOVERY_MODE_BRIDGE_ANNOUNCED:
		return domainfederation.DiscoveryModeBridgeAnnounced
	default:
		return domainfederation.DiscoveryModeUnspecified
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

func mediaPolicyFromProto(value federationv1.MediaPolicy) domainfederation.MediaPolicy {
	switch value {
	case federationv1.MediaPolicy_MEDIA_POLICY_REFERENCE_PROXY:
		return domainfederation.MediaPolicyReferenceProxy
	case federationv1.MediaPolicy_MEDIA_POLICY_BACKGROUND_REPLICATION:
		return domainfederation.MediaPolicyBackgroundReplication
	case federationv1.MediaPolicy_MEDIA_POLICY_DISABLED:
		return domainfederation.MediaPolicyDisabled
	default:
		return domainfederation.MediaPolicyUnspecified
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

func linkStateFromProto(value federationv1.LinkState) domainfederation.LinkState {
	switch value {
	case federationv1.LinkState_LINK_STATE_ACTIVE:
		return domainfederation.LinkStateActive
	case federationv1.LinkState_LINK_STATE_PAUSED:
		return domainfederation.LinkStatePaused
	case federationv1.LinkState_LINK_STATE_DEGRADED:
		return domainfederation.LinkStateDegraded
	case federationv1.LinkState_LINK_STATE_DELETED:
		return domainfederation.LinkStateDeleted
	default:
		return domainfederation.LinkStateUnspecified
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

func conversationKindsFromProto(values []commonv1.ConversationKind) []domainfederation.ConversationKind {
	if len(values) == 0 {
		return nil
	}

	converted := make([]domainfederation.ConversationKind, 0, len(values))
	for _, value := range values {
		switch value {
		case commonv1.ConversationKind_CONVERSATION_KIND_DIRECT:
			converted = append(converted, domainfederation.ConversationKindDirect)
		case commonv1.ConversationKind_CONVERSATION_KIND_GROUP:
			converted = append(converted, domainfederation.ConversationKindGroup)
		case commonv1.ConversationKind_CONVERSATION_KIND_CHANNEL:
			converted = append(converted, domainfederation.ConversationKindChannel)
		}
	}

	return converted
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

func bundleStateProto(value domainfederation.BundleState) federationv1.BundleState {
	switch value {
	case domainfederation.BundleStateQueued:
		return federationv1.BundleState_BUNDLE_STATE_QUEUED
	case domainfederation.BundleStateAccepted:
		return federationv1.BundleState_BUNDLE_STATE_ACCEPTED
	case domainfederation.BundleStateAcknowledged:
		return federationv1.BundleState_BUNDLE_STATE_ACKNOWLEDGED
	case domainfederation.BundleStateFailed:
		return federationv1.BundleState_BUNDLE_STATE_FAILED
	default:
		return federationv1.BundleState_BUNDLE_STATE_UNSPECIFIED
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

	return timestamppb.New(value)
}

func timestampValue(value *timestamppb.Timestamp) time.Time {
	if value == nil {
		return time.Time{}
	}

	return value.AsTime().UTC()
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}

	return right
}

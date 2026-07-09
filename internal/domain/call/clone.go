package call

func cloneMediaState(state MediaState) MediaState {
	return state
}

func cloneTransportStats(stats TransportStats) TransportStats {
	stats.RecentSamples = cloneTransportQualitySamples(stats.RecentSamples)
	stats.RecentQoSSamples = cloneTransportQoSSamples(stats.RecentQoSSamples)
	return stats
}

func cloneQualitySummary(summary QualitySummary) QualitySummary {
	return summary
}

func cloneDiagnostics(value Diagnostics) Diagnostics {
	value.Call = cloneCall(value.Call)
	return value
}

func cloneTransportQualitySample(sample TransportQualitySample) TransportQualitySample {
	return sample
}

func cloneTransportQualitySamples(samples []TransportQualitySample) []TransportQualitySample {
	if len(samples) == 0 {
		return nil
	}

	cloned := make([]TransportQualitySample, len(samples))
	for i := range samples {
		cloned[i] = cloneTransportQualitySample(samples[i])
	}

	return cloned
}

func cloneTransportQoSSample(sample TransportQoSSample) TransportQoSSample {
	return sample
}

func cloneTransportQoSSamples(samples []TransportQoSSample) []TransportQoSSample {
	if len(samples) == 0 {
		return nil
	}

	cloned := make([]TransportQoSSample, len(samples))
	for i := range samples {
		cloned[i] = cloneTransportQoSSample(samples[i])
	}

	return cloned
}

func cloneInvite(invite Invite) Invite {
	return invite
}

func cloneInvites(invites []Invite) []Invite {
	if len(invites) == 0 {
		return nil
	}

	cloned := make([]Invite, len(invites))
	for i := range invites {
		cloned[i] = cloneInvite(invites[i])
	}

	return cloned
}

func cloneParticipant(participant Participant) Participant {
	participant.MediaState = cloneMediaState(participant.MediaState)
	participant.Transport = cloneTransportStats(participant.Transport)
	return participant
}

func cloneParticipants(participants []Participant) []Participant {
	if len(participants) == 0 {
		return nil
	}

	cloned := make([]Participant, len(participants))
	for i := range participants {
		cloned[i] = cloneParticipant(participants[i])
	}

	return cloned
}

func cloneCall(value Call) Call {
	value.Invites = cloneInvites(value.Invites)
	value.Participants = cloneParticipants(value.Participants)
	value.QualitySummary = cloneQualitySummary(value.QualitySummary)
	return value
}

func cloneCalls(values []Call) []Call {
	if len(values) == 0 {
		return nil
	}

	cloned := make([]Call, len(values))
	for i := range values {
		cloned[i] = cloneCall(values[i])
	}

	return cloned
}

func cloneStringMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}

	return dst
}

func cloneEvent(event Event) Event {
	event.Metadata = cloneStringMetadata(event.Metadata)
	event.Call = cloneCall(event.Call)
	return event
}

func cloneEvents(events []Event) []Event {
	if len(events) == 0 {
		return nil
	}

	cloned := make([]Event, len(events))
	for i := range events {
		cloned[i] = cloneEvent(events[i])
	}

	return cloned
}

func cloneIceServers(src []IceServer) []IceServer {
	if len(src) == 0 {
		return nil
	}

	dst := make([]IceServer, len(src))
	for i := range src {
		dst[i] = IceServer{
			URLs:       append([]string(nil), src[i].URLs...),
			Username:   src[i].Username,
			Credential: src[i].Credential,
			ExpiresAt:  src[i].ExpiresAt,
		}
	}

	return dst
}

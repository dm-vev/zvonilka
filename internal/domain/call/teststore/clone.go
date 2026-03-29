package teststore

import "github.com/dm-vev/zvonilka/internal/domain/call"

func (s *memoryStore) cloneLocked() memoryStore {
	return memoryStore{
		callsByID:         cloneCalls(s.callsByID),
		invitesByKey:      cloneInvites(s.invitesByKey),
		participantsByKey: cloneParticipants(s.participantsByKey),
		eventsByID:        cloneEvents(s.eventsByID),
		workerCursors:     cloneWorkerCursors(s.workerCursors),
		eventOrder:        append([]string(nil), s.eventOrder...),
		nextSequence:      s.nextSequence,
	}
}

func (s *memoryStore) replaceLocked(tx *memoryStore) {
	s.callsByID = cloneCalls(tx.callsByID)
	s.invitesByKey = cloneInvites(tx.invitesByKey)
	s.participantsByKey = cloneParticipants(tx.participantsByKey)
	s.eventsByID = cloneEvents(tx.eventsByID)
	s.workerCursors = cloneWorkerCursors(tx.workerCursors)
	s.eventOrder = append([]string(nil), tx.eventOrder...)
	s.nextSequence = tx.nextSequence
}

func cloneCalls(src map[string]call.Call) map[string]call.Call {
	if len(src) == 0 {
		return make(map[string]call.Call)
	}

	dst := make(map[string]call.Call, len(src))
	for key, value := range src {
		dst[key] = callClone(value)
	}

	return dst
}

func cloneInvites(src map[string]call.Invite) map[string]call.Invite {
	if len(src) == 0 {
		return make(map[string]call.Invite)
	}

	dst := make(map[string]call.Invite, len(src))
	for key, value := range src {
		dst[key] = value
	}

	return dst
}

func cloneParticipants(src map[string]call.Participant) map[string]call.Participant {
	if len(src) == 0 {
		return make(map[string]call.Participant)
	}

	dst := make(map[string]call.Participant, len(src))
	for key, value := range src {
		dst[key] = value
	}

	return dst
}

func cloneEvents(src map[string]call.Event) map[string]call.Event {
	if len(src) == 0 {
		return make(map[string]call.Event)
	}

	dst := make(map[string]call.Event, len(src))
	for key, value := range src {
		dst[key] = eventClone(value)
	}

	return dst
}

func cloneWorkerCursors(src map[string]call.WorkerCursor) map[string]call.WorkerCursor {
	if len(src) == 0 {
		return make(map[string]call.WorkerCursor)
	}

	dst := make(map[string]call.WorkerCursor, len(src))
	for key, value := range src {
		dst[key] = value
	}

	return dst
}

func callClone(value call.Call) call.Call {
	value.Invites = append([]call.Invite(nil), value.Invites...)
	value.Participants = append([]call.Participant(nil), value.Participants...)
	return value
}

func eventClone(value call.Event) call.Event {
	if len(value.Metadata) > 0 {
		cloned := make(map[string]string, len(value.Metadata))
		for key, item := range value.Metadata {
			cloned[key] = item
		}
		value.Metadata = cloned
	}
	value.Call = callClone(value.Call)
	return value
}

package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/notification"
)

func (s *Store) SaveScopeSettings(ctx context.Context, settings notification.ScopeSettings) (notification.ScopeSettings, error) {
	settings, err := notification.NormalizeScopeSettings(settings, time.Now().UTC())
	if err != nil {
		return notification.ScopeSettings{}, err
	}
	query := fmt.Sprintf(`INSERT INTO %s (account_id, scope, muted_until, show_preview, sound_id, mute_stories, story_sound_id, show_story_sender, disable_pinned_message_notifications, disable_mention_notifications, use_default_mute_stories, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (account_id, scope) DO UPDATE SET muted_until=EXCLUDED.muted_until, show_preview=EXCLUDED.show_preview, sound_id=EXCLUDED.sound_id, mute_stories=EXCLUDED.mute_stories, story_sound_id=EXCLUDED.story_sound_id, show_story_sender=EXCLUDED.show_story_sender, disable_pinned_message_notifications=EXCLUDED.disable_pinned_message_notifications, disable_mention_notifications=EXCLUDED.disable_mention_notifications, use_default_mute_stories=EXCLUDED.use_default_mute_stories, updated_at=EXCLUDED.updated_at
RETURNING account_id, scope, muted_until, show_preview, sound_id, mute_stories, story_sound_id, show_story_sender, disable_pinned_message_notifications, disable_mention_notifications, use_default_mute_stories, updated_at`, s.table("notification_scope_settings"))
	return scanScopeSettings(s.conn().QueryRowContext(ctx, query, settings.AccountID, settings.Scope, encodeTime(settings.MutedUntil), settings.ShowPreview, settings.SoundID, settings.MuteStories, settings.StorySoundID, settings.ShowStorySender, settings.DisablePinnedMessageNotifications, settings.DisableMentionNotifications, settings.UseDefaultMuteStories, settings.UpdatedAt.UTC()))
}

func (s *Store) ScopeSettingsByAccountAndScope(ctx context.Context, accountID string, scope notification.SettingsScope) (notification.ScopeSettings, error) {
	query := fmt.Sprintf(`SELECT account_id, scope, muted_until, show_preview, sound_id, mute_stories, story_sound_id, show_story_sender, disable_pinned_message_notifications, disable_mention_notifications, use_default_mute_stories, updated_at FROM %s WHERE account_id=$1 AND scope=$2`, s.table("notification_scope_settings"))
	settings, err := scanScopeSettings(s.conn().QueryRowContext(ctx, query, strings.TrimSpace(accountID), scope))
	if errors.Is(err, sql.ErrNoRows) {
		return notification.ScopeSettings{}, notification.ErrNotFound
	}
	return settings, err
}

func (s *Store) SaveReactionSettings(ctx context.Context, settings notification.ReactionSettings) (notification.ReactionSettings, error) {
	settings, err := notification.NormalizeReactionSettings(settings, time.Now().UTC())
	if err != nil {
		return notification.ReactionSettings{}, err
	}
	query := fmt.Sprintf(`INSERT INTO %s (account_id, message_reaction_source, story_reaction_source, poll_vote_source, sound_id, show_preview, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (account_id) DO UPDATE SET message_reaction_source=EXCLUDED.message_reaction_source, story_reaction_source=EXCLUDED.story_reaction_source, poll_vote_source=EXCLUDED.poll_vote_source, sound_id=EXCLUDED.sound_id, show_preview=EXCLUDED.show_preview, updated_at=EXCLUDED.updated_at
RETURNING account_id, message_reaction_source, story_reaction_source, poll_vote_source, sound_id, show_preview, updated_at`, s.table("notification_reaction_settings"))
	return scanReactionSettings(s.conn().QueryRowContext(ctx, query, settings.AccountID, settings.MessageReactionSource, settings.StoryReactionSource, settings.PollVoteSource, settings.SoundID, settings.ShowPreview, settings.UpdatedAt.UTC()))
}

func (s *Store) ReactionSettingsByAccountID(ctx context.Context, accountID string) (notification.ReactionSettings, error) {
	query := fmt.Sprintf(`SELECT account_id, message_reaction_source, story_reaction_source, poll_vote_source, sound_id, show_preview, updated_at FROM %s WHERE account_id=$1`, s.table("notification_reaction_settings"))
	settings, err := scanReactionSettings(s.conn().QueryRowContext(ctx, query, strings.TrimSpace(accountID)))
	if errors.Is(err, sql.ErrNoRows) {
		return notification.ReactionSettings{}, notification.ErrNotFound
	}
	return settings, err
}

func (s *Store) SaveSavedSound(ctx context.Context, sound notification.SavedSound) (notification.SavedSound, error) {
	sound, err := notification.NormalizeSavedSound(sound, time.Now().UTC())
	if err != nil {
		return notification.SavedSound{}, err
	}
	query := fmt.Sprintf(`INSERT INTO %s (account_id, media_id, title, created_at) VALUES ($1,$2,$3,$4)
ON CONFLICT (account_id, media_id) DO UPDATE SET title=EXCLUDED.title
RETURNING sound_id, account_id, media_id, title, created_at`, s.table("notification_saved_sounds"))
	return scanSavedSound(s.conn().QueryRowContext(ctx, query, sound.AccountID, sound.MediaID, sound.Title, sound.CreatedAt.UTC()))
}

func (s *Store) SavedSoundsByAccountID(ctx context.Context, accountID string) ([]notification.SavedSound, error) {
	query := fmt.Sprintf(`SELECT sound_id, account_id, media_id, title, created_at FROM %s WHERE account_id=$1 ORDER BY created_at ASC, sound_id ASC`, s.table("notification_saved_sounds"))
	rows, err := s.conn().QueryContext(ctx, query, strings.TrimSpace(accountID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sounds []notification.SavedSound
	for rows.Next() {
		sound, scanErr := scanSavedSound(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		sounds = append(sounds, sound)
	}
	return sounds, rows.Err()
}

func (s *Store) DeleteSavedSound(ctx context.Context, accountID string, soundID int64) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE account_id=$1 AND sound_id=$2`, s.table("notification_saved_sounds"))
	result, err := s.conn().ExecContext(ctx, query, strings.TrimSpace(accountID), soundID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return notification.ErrNotFound
	}
	return nil
}

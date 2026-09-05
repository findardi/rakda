package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/findardi/rakda/server/internal/platform/ptr"
	"io"
	"log"
	"path"
	"strings"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/platform/brandimage"
	"github.com/findardi/rakda/server/internal/workspace/dto"
	workspacedb "github.com/findardi/rakda/server/internal/workspace/repository/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// MaxLogoUploadBytes bounds what a browser may send; the stored PNG is far
	// smaller because the picture is re-encoded at logoMaxEdge.
	MaxLogoUploadBytes = 2 << 20
	logoMaxEdge        = 512
	logoContentType    = "image/png"

	// logoPrefix is a top-level family like downloads/ and archives/, kept
	// apart from anything an age-based sweeper walks: a logo is long-lived
	// state, never a cache.
	logoPrefix = "asset/logo/"
)

var (
	ErrLogoNotFound      = errors.New("workspace has no logo")
	ErrLogoTooLarge      = errors.New("logo exceeds the 2 MB limit")
	ErrLogoUnsupported   = errors.New("logo must be a PNG, JPEG, or WebP image")
	ErrLogoInvalid       = errors.New("logo image cannot be read or has unusable dimensions")
	ErrHeroPresetInvalid = errors.New("unknown hero preset")
)

// HeroPreset is one curated hero identity: a hue on the oklch wheel and an arc
// phase. This list is the single source of truth — the web renders from it and
// the service validates against it — so a preset can never exist on one side
// only. Lightness and chroma stay fixed in the renderer, which is what keeps a
// room's choice from competing with the information on the page.
type HeroPreset struct {
	Key   string
	Hue   int
	Phase int
}

var heroPresets = []HeroPreset{
	{Key: "tide", Hue: 190, Phase: 0},
	{Key: "reef", Hue: 202, Phase: 12},
	{Key: "lagoon", Hue: 214, Phase: 24},
	{Key: "slate", Hue: 226, Phase: 36},
	{Key: "harbor", Hue: 238, Phase: 8},
	{Key: "moss", Hue: 160, Phase: 20},
	{Key: "olive", Hue: 130, Phase: 30},
	{Key: "clay", Hue: 40, Phase: 16},
	{Key: "plum", Hue: 320, Phase: 28},
	{Key: "ember", Hue: 20, Phase: 4},
}

func heroPresetByKey(key string) (HeroPreset, bool) {
	for _, p := range heroPresets {
		if p.Key == key {
			return p, true
		}
	}
	return HeroPreset{}, false
}

// autoHero is the identity every room is born with, derived from the slug so a
// room keeps one face for its whole life without anyone choosing. It mirrors
// the hash the web used before presets existed, so no existing room changes
// colour because of this feature.
func autoHero(slug string) (hue, phase int) {
	var h uint32
	for _, c := range slug {
		h = h*31 + uint32(c)
	}
	return 190 + int(h%5)*12, int(h % 40)
}

// applyBranding fills the branding fields of a response from the row. A stored
// preset that no longer exists in the list falls back to the automatic
// identity rather than failing the whole room.
func applyBranding(res *dto.WorkspaceResponse, slug string, logoKey, heroPreset *string) {
	res.Logo = logoVersion(logoKey)
	if heroPreset != nil {
		if p, ok := heroPresetByKey(*heroPreset); ok {
			res.HeroPreset, res.HeroHue, res.HeroPhase = p.Key, p.Hue, p.Phase
			return
		}
	}
	res.HeroHue, res.HeroPhase = autoHero(slug)
}

// logoVersion is the uuid segment of the object key. It changes on every
// upload, so it doubles as the token the logo endpoint is fetched and cached
// with — an old URL can never show a new picture, or the other way round.
func logoVersion(key *string) string {
	if key == nil {
		return ""
	}
	base := path.Base(*key)
	return strings.TrimSuffix(base, path.Ext(base))
}

func logoObjectKey(workspaceID string) string {
	return fmt.Sprintf("%s%s/%s.png", logoPrefix, workspaceID, uuid.NewString())
}

func logoObjectPrefix(workspaceID string) string {
	return logoPrefix + workspaceID + "/"
}

func (s *WorkspaceService) HeroPresets() []dto.HeroPresetResponse {
	out := make([]dto.HeroPresetResponse, 0, len(heroPresets))
	for _, p := range heroPresets {
		out = append(out, dto.HeroPresetResponse{Key: p.Key, Hue: p.Hue, Phase: p.Phase})
	}
	return out
}

// writableWorkspace loads a room for a branding mutation: it must exist and
// must not be archived — the same in-service 423 DeleteWorkspace uses, since
// the workspace routes carry no membership resolver for RequireRoomWritable.
func (s *WorkspaceService) writableWorkspace(ctx context.Context, workspaceID string) (pgtype.UUID, workspacedb.Workspace, error) {
	var uid pgtype.UUID
	if err := uid.Scan(workspaceID); err != nil {
		return uid, workspacedb.Workspace{}, fmt.Errorf("parse workspace id: %w", err)
	}

	current, err := s.repo.GetWorkspaceByID(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return uid, workspacedb.Workspace{}, ErrWorkspaceNotFound
	}
	if err != nil {
		return uid, workspacedb.Workspace{}, fmt.Errorf("get workspace: %w", err)
	}
	if current.Status == StatusArchive {
		return uid, workspacedb.Workspace{}, ErrWorkspaceArchived
	}

	return uid, current, nil
}

func (s *WorkspaceService) brandingEntry(workspaceID string, actor Actor, name string, meta map[string]any) activityservice.Entry {
	return activityservice.Entry{
		WorkspaceID: workspaceID,
		ActorID:     actor.UserID,
		ActorName:   actor.Name,
		ActorRole:   actor.Role,
		Action:      activityservice.ActionWorkspaceBrandingChanged,
		TargetType:  activityservice.TargetWorkspace,
		TargetID:    workspaceID,
		TargetName:  name,
		Metadata:    meta,
	}
}

// SetLogo normalizes the uploaded picture and makes it the room's logo.
func (s *WorkspaceService) SetLogo(ctx context.Context, workspaceID string, actor Actor, r io.Reader) (dto.WorkspaceResponse, error) {
	uid, current, err := s.writableWorkspace(ctx, workspaceID)
	if err != nil {
		return dto.WorkspaceResponse{}, err
	}

	logo, err := brandimage.NormalizeLogo(r, MaxLogoUploadBytes, logoMaxEdge)
	switch {
	case errors.Is(err, brandimage.ErrTooLarge):
		return dto.WorkspaceResponse{}, ErrLogoTooLarge
	case errors.Is(err, brandimage.ErrUnsupported):
		return dto.WorkspaceResponse{}, ErrLogoUnsupported
	case errors.Is(err, brandimage.ErrInvalid), errors.Is(err, brandimage.ErrDimensions):
		return dto.WorkspaceResponse{}, ErrLogoInvalid
	case err != nil:
		return dto.WorkspaceResponse{}, fmt.Errorf("normalize logo: %w", err)
	}

	// The object is written before the row points at it and removed again if
	// the row never does: a row must never reference a key that is not there.
	key := logoObjectKey(workspaceID)
	if err := s.store.Put(ctx, key, bytes.NewReader(logo), int64(len(logo)), logoContentType); err != nil {
		return dto.WorkspaceResponse{}, fmt.Errorf("store logo: %w", err)
	}

	updated, err := s.setLogoKey(ctx, uid, workspaceID, &key, actor, "set")
	if err != nil {
		s.discardObject(ctx, key)
		return dto.WorkspaceResponse{}, err
	}

	// The previous picture is unreachable once the row moved on; a failed
	// delete here leaks one small object, never a broken page.
	if current.LogoKey != nil {
		s.discardObject(ctx, *current.LogoKey)
	}

	return workspaceResponse(updated), nil
}

// RemoveLogo returns the room to its generative identity. Removing a logo that
// is not there succeeds silently and records nothing.
func (s *WorkspaceService) RemoveLogo(ctx context.Context, workspaceID string, actor Actor) (dto.WorkspaceResponse, error) {
	uid, current, err := s.writableWorkspace(ctx, workspaceID)
	if err != nil {
		return dto.WorkspaceResponse{}, err
	}
	if current.LogoKey == nil {
		return workspaceResponse(current), nil
	}

	updated, err := s.setLogoKey(ctx, uid, workspaceID, nil, actor, "removed")
	if err != nil {
		return dto.WorkspaceResponse{}, err
	}
	s.discardObject(ctx, *current.LogoKey)

	return workspaceResponse(updated), nil
}

func (s *WorkspaceService) setLogoKey(ctx context.Context, uid pgtype.UUID, workspaceID string, key *string, actor Actor, action string) (workspacedb.Workspace, error) {
	var updated workspacedb.Workspace
	err := s.repo.ExecTx(ctx, func(q *workspacedb.Queries, tx pgx.Tx) error {
		w, err := q.UpdateWorkspaceLogo(ctx, workspacedb.UpdateWorkspaceLogoParams{ID: uid, LogoKey: key})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrWorkspaceNotFound
		}
		if err != nil {
			return fmt.Errorf("update logo: %w", err)
		}

		if err := s.activity.RecordTx(ctx, tx, s.brandingEntry(workspaceID, actor, w.Name,
			map[string]any{"kind": "logo", "action": action})); err != nil {
			return err
		}

		updated = w
		return nil
	})
	return updated, err
}

// OpenLogo streams a room's logo to a member. Anyone who can enter the room
// may see its logo; nobody else learns whether one exists (the same not-found
// either way). When the caller already holds knownVersion, nothing is opened
// and a nil reader comes back with the version — the handler's 304.
func (s *WorkspaceService) OpenLogo(ctx context.Context, workspaceID, userID, knownVersion string) (io.ReadCloser, string, error) {
	var wid, uid pgtype.UUID
	if err := wid.Scan(workspaceID); err != nil {
		return nil, "", fmt.Errorf("parse workspace id: %w", err)
	}
	if err := uid.Scan(userID); err != nil {
		return nil, "", fmt.Errorf("parse user id: %w", err)
	}

	w, err := s.repo.GetWorkspaceForMember(ctx, workspacedb.GetWorkspaceForMemberParams{
		WorkspaceID: wid,
		UserID:      uid,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrWorkspaceNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("get workspace: %w", err)
	}
	if w.LogoKey == nil {
		return nil, "", ErrLogoNotFound
	}

	version := logoVersion(w.LogoKey)
	if knownVersion != "" && knownVersion == version {
		return nil, version, nil
	}

	rc, err := s.store.Get(ctx, *w.LogoKey)
	if err != nil {
		return nil, "", fmt.Errorf("get logo: %w", err)
	}
	return rc, version, nil
}

// SetHeroPreset picks a curated hero identity; an empty preset returns the
// room to the automatic one.
func (s *WorkspaceService) SetHeroPreset(ctx context.Context, req dto.WorkspaceHeroRequest, actor Actor) (dto.WorkspaceResponse, error) {
	uid, _, err := s.writableWorkspace(ctx, req.ID)
	if err != nil {
		return dto.WorkspaceResponse{}, err
	}

	var preset *string
	if key := strings.TrimSpace(req.Preset); key != "" {
		p, ok := heroPresetByKey(key)
		if !ok {
			return dto.WorkspaceResponse{}, ErrHeroPresetInvalid
		}
		preset = &p.Key
	}

	var updated workspacedb.Workspace
	err = s.repo.ExecTx(ctx, func(q *workspacedb.Queries, tx pgx.Tx) error {
		w, err := q.UpdateWorkspaceHeroPreset(ctx, workspacedb.UpdateWorkspaceHeroPresetParams{ID: uid, HeroPreset: preset})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrWorkspaceNotFound
		}
		if err != nil {
			return fmt.Errorf("update hero preset: %w", err)
		}

		if err := s.activity.RecordTx(ctx, tx, s.brandingEntry(req.ID, actor, w.Name,
			map[string]any{"kind": "hero", "preset": ptr.Deref(preset)})); err != nil {
			return err
		}

		updated = w
		return nil
	})
	if err != nil {
		return dto.WorkspaceResponse{}, err
	}

	return workspaceResponse(updated), nil
}

// discardObject removes an object nothing references any more. Failure is
// logged, not returned: the row is already correct, and a sweeper for asset/
// orphans is not worth the one small object this could leak.
func (s *WorkspaceService) discardObject(ctx context.Context, key string) {
	if err := s.store.Delete(ctx, key); err != nil {
		log.Printf("workspace: delete object %s: %v", key, err)
	}
}

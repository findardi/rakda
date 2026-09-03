-- +goose Up
-- +goose StatementBegin
-- Optional access expiry for guest members. NULL = never expires. The lapse
-- is derived at query time (nothing flips a stored status), so the member
-- row survives and can be extended with a single UPDATE.
alter table workspace_members
    add column expires_at timestamptz;

-- The date chosen at invite time, copied to the member on acceptance. Named
-- apart from expires_at, which is the invitation link's own validity window.
alter table workspace_user_invitations
    add column access_expires_at timestamptz;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
alter table workspace_user_invitations
    drop column access_expires_at;

alter table workspace_members
    drop column expires_at;
-- +goose StatementEnd

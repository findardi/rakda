-- +goose Up
-- +goose StatementBegin
-- Room branding, both optional. logo_key is the object key of the re-encoded
-- PNG under asset/logo/{workspace}/; hero_preset is the key of a curated hero
-- identity validated in Go. NULL keeps today's generative default for both.
alter table workspaces
    add column logo_key text,
    add column hero_preset text;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
alter table workspaces
    drop column hero_preset,
    drop column logo_key;
-- +goose StatementEnd

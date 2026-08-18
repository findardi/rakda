-- +goose Up
update folder_access
set can_download_original = false
where can_watermark and can_download_original;

alter table folder_access
    add constraint folder_access_mark_excludes_clean_download
        check (not (can_watermark and can_download_original));

-- +goose Down
alter table folder_access
    drop constraint folder_access_mark_excludes_clean_download;

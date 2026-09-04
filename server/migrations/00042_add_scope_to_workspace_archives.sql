-- +goose Up
-- +goose StatementBegin
-- Cakupan opsional sebuah paket arsip. Kedua kolom NULL = seluruh ruang
-- (perilaku lama, dan setiap baris yang sudah ada). scope_folder_ids berisi
-- folder yang subtree-nya disertakan; scope_folder_names adalah potret nama
-- saat pembuatan, urutan sama, supaya daftar tetap bisa menampilkan cakupan
-- setelah folder diganti nama atau dihapus. Sengaja tanpa FK: paket hidup
-- lebih lama dari foldernya.
alter table workspace_archives
    add column scope_folder_ids uuid[],
    add column scope_folder_names text[];

-- Keduanya NULL, atau keduanya terisi dan sejajar. '{}' ditolak agar salah
-- nil-vs-kosong di Go gagal keras, bukan menghasilkan paket kosong.
alter table workspace_archives
    add constraint workspace_archives_scope_check check (
        (scope_folder_ids is null and scope_folder_names is null)
        or (cardinality(scope_folder_ids) >= 1
            and cardinality(scope_folder_ids) = cardinality(scope_folder_names))
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
alter table workspace_archives
    drop constraint workspace_archives_scope_check,
    drop column scope_folder_names,
    drop column scope_folder_ids;
-- +goose StatementEnd

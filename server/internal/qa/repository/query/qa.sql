-- name: GetMemberGroupQA :one
select g.id, g.name, g.qa_enabled, g.qa_question_limit
from workspace_group_members gm
join workspace_members m on m.id = gm.member_id
join workspace_groups g on g.id = gm.group_id
where m.workspace_id = @workspace_id and m.user_id = @user_id;

-- name: LockGroupQA :one
select id, name, qa_enabled, qa_question_limit
from workspace_groups
where id = @group_id and workspace_id = @workspace_id
for update;

-- name: GroupQuestionStats :one
select
    count(*)::bigint as used,
    (coalesce(max(number), 0) + 1)::int as next_number
from questions
where group_id = @group_id;

-- name: InsertQuestion :one
insert into questions
    (workspace_id, group_id, group_name, number, author_id, author_name,
     subject, body, document_id, document_name, folder_id, folder_name)
values
    (@workspace_id, @group_id, @group_name, @number, @author_id, @author_name,
     @subject, @body, @document_id, @document_name, @folder_id, @folder_name)
returning *;

-- name: ListQuestions :many
select
    q.*,
    (select count(*) from question_replies qr where qr.question_id = q.id)::bigint as reply_count
from questions q
where q.workspace_id = @workspace_id
    and (sqlc.narg('group_id')::uuid is null or q.group_id = sqlc.narg('group_id'))
    and (sqlc.narg('status')::text is null or q.status = sqlc.narg('status'))
    and (sqlc.narg('cursor_created_at')::timestamptz is null
        or (q.created_at, q.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
order by q.created_at desc, q.id desc
limit @page_size;

-- name: CountQuestions :one
select count(*) from questions
where workspace_id = @workspace_id
    and (sqlc.narg('group_id')::uuid is null or group_id = sqlc.narg('group_id'));

-- name: CountWaitingQuestions :one
select count(*) from questions
where workspace_id = @workspace_id and status = 'waiting';

-- name: GetQuestion :one
select * from questions
where id = @id and workspace_id = @workspace_id;

-- name: GetQuestionLocked :one
select * from questions
where id = @id
for update;

-- name: UpdateQuestionStatus :exec
update questions set status = @status where id = @id;

-- name: ListQuestionReplies :many
select * from question_replies
where question_id = @question_id
order by created_at asc, id asc;

-- name: InsertReply :one
insert into question_replies
    (question_id, author_id, author_name, author_role, body)
values
    (@question_id, @author_id, @author_name, @author_role, @body)
returning *;

-- name: ListFaqs :many
select * from faqs
where workspace_id = @workspace_id
order by created_at desc, id desc;

-- name: CountFaqs :one
select count(*) from faqs
where workspace_id = @workspace_id;

-- name: InsertFaq :one
insert into faqs
    (workspace_id, question_text, answer_text, source_question_id, created_by, creator_name)
values
    (@workspace_id, @question_text, @answer_text, @source_question_id, @created_by, @creator_name)
returning *;

-- name: GetDocumentForRef :one
select id, workspace_id, folder_id, name from documents
where id = @id and deleted_at is null;

-- name: GetFolderForRef :one
select id, workspace_id, name from folders
where id = @id and deleted_at is null;

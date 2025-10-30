drop index if exists idx_attachments_announcement_id;
drop index if exists idx_attachments_name;
drop index if exists idx_attachments_path;
drop index if exists idx_attachments_created_at;

drop table if exists attachments;

drop type if exists attachment_type;

DROP TABLE buddy.vault_versions;
DROP TABLE buddy.vault_items;
DELETE FROM buddy.threads WHERE type_id = 5;
DELETE FROM buddy.thread_types WHERE id = 5;

\set ON_ERROR_STOP on

DO $$
DECLARE
    table_list text;
BEGIN
    SELECT string_agg(format('%I.%I', n.nspname, c.relname), ', ' ORDER BY n.nspname, c.relname)
    INTO table_list
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relkind IN ('r', 'p')
      AND c.relname <> 'oauth_credential_vault';

    IF table_list IS NULL THEN
        RAISE EXCEPTION 'No public business tables found';
    END IF;

    IF EXISTS (SELECT 1 FROM pg_publication WHERE pubname = 'anl_core_publication') THEN
        EXECUTE 'ALTER PUBLICATION anl_core_publication SET TABLE ' || table_list;
    ELSE
        EXECUTE 'CREATE PUBLICATION anl_core_publication FOR TABLE ' || table_list;
    END IF;
END $$;

SELECT 'publication_tables=' || count(*)
FROM pg_publication_tables
WHERE pubname = 'anl_core_publication';

SELECT CASE WHEN count(*) = 0 THEN 'publication_oauth_vault_excluded=true'
            ELSE 'publication_oauth_vault_excluded=false' END
FROM pg_publication_tables
WHERE pubname = 'anl_core_publication'
  AND tablename = 'oauth_credential_vault';

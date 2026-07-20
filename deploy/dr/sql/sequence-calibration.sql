\set ON_ERROR_STOP on

DO $$
DECLARE
    item record;
    maximum_value bigint;
BEGIN
    FOR item IN
        SELECT
            seq_ns.nspname AS sequence_schema,
            seq.relname AS sequence_name,
            table_ns.nspname AS table_schema,
            tbl.relname AS table_name,
            attr.attname AS column_name,
            pg_sequence.seqstart AS start_value
        FROM pg_class seq
        JOIN pg_namespace seq_ns ON seq_ns.oid = seq.relnamespace
        JOIN pg_sequence ON pg_sequence.seqrelid = seq.oid
        JOIN pg_depend dep ON dep.objid = seq.oid AND dep.deptype IN ('a', 'i')
        JOIN pg_class tbl ON tbl.oid = dep.refobjid
        JOIN pg_namespace table_ns ON table_ns.oid = tbl.relnamespace
        JOIN pg_attribute attr ON attr.attrelid = tbl.oid AND attr.attnum = dep.refobjsubid
        WHERE seq.relkind = 'S'
          AND seq_ns.nspname = 'public'
    LOOP
        EXECUTE format('SELECT max(%I)::bigint FROM %I.%I', item.column_name, item.table_schema, item.table_name)
        INTO maximum_value;
        IF maximum_value IS NULL THEN
            PERFORM setval(
                format('%I.%I', item.sequence_schema, item.sequence_name)::regclass,
                item.start_value,
                false
            );
        ELSE
            PERFORM setval(
                format('%I.%I', item.sequence_schema, item.sequence_name)::regclass,
                GREATEST(maximum_value, item.start_value),
                true
            );
        END IF;
    END LOOP;
END $$;

SELECT 'calibrated_sequences=' || count(*)
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'S' AND n.nspname = 'public';

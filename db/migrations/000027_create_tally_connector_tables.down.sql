BEGIN;

DROP TABLE IF EXISTS tally_vouchers;
DROP TABLE IF EXISTS sync_events;
DROP TABLE IF EXISTS tally_cost_centres;
DROP TABLE IF EXISTS tally_units;
DROP TABLE IF EXISTS tally_godowns;
DROP TABLE IF EXISTS tally_stock_items;
DROP TABLE IF EXISTS tally_ledgers;
DROP TABLE IF EXISTS connector_agents;

COMMIT;

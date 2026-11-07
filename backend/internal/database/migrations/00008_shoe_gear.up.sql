-- 4.1.0: Strava gear metadata on shoes (real mileage golden source, primary pair, retired or
-- deleted on Strava). strava_state: NULL (never synced) | active | retired | deleted.

ALTER TABLE shoes ADD COLUMN strava_primary INTEGER NOT NULL DEFAULT 0;
ALTER TABLE shoes ADD COLUMN strava_state TEXT;
ALTER TABLE shoes ADD COLUMN strava_synced_at INTEGER;

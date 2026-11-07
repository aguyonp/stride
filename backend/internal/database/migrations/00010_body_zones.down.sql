-- Back to the six 4.0 zones: entries of the four added zones cannot satisfy the old CHECK and are
-- dropped, every other row is copied as is. The added zones are also removed from
-- health_status.zones (older code would reject them); conditions on the other zones are kept
-- (ignored by older code, back on a re-upgrade).

CREATE TABLE body_stress_entries_v6 (
    id                  INTEGER PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date                TEXT    NOT NULL,
    activity_id         INTEGER REFERENCES activities(id) ON DELETE SET NULL,
    zone                TEXT    NOT NULL CHECK (zone IN ('knee_pfps', 'achilles', 'plantar_fascia', 'shin_splints', 'iliotibial_band', 'hamstrings_calves')),
    side                TEXT    NOT NULL DEFAULT 'both' CHECK (side IN ('left', 'right', 'both')),
    pain_score          REAL    NOT NULL CHECK (pain_score >= 0 AND pain_score <= 10),
    cold_next_day_score REAL    CHECK (cold_next_day_score IS NULL OR (cold_next_day_score >= 0 AND cold_next_day_score <= 10)),
    note                TEXT    NOT NULL DEFAULT '',
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL
);
INSERT INTO body_stress_entries_v6 (id, user_id, date, activity_id, zone, side, pain_score, cold_next_day_score, note, created_at, updated_at)
SELECT id, user_id, date, activity_id, zone, side, pain_score, cold_next_day_score, note, created_at, updated_at FROM body_stress_entries
WHERE zone IN ('knee_pfps', 'achilles', 'plantar_fascia', 'shin_splints', 'iliotibial_band', 'hamstrings_calves');
DROP INDEX IF EXISTS body_stress_entries_activity;
DROP INDEX IF EXISTS body_stress_entries_user_date;
DROP TABLE body_stress_entries;
ALTER TABLE body_stress_entries_v6 RENAME TO body_stress_entries;
CREATE INDEX body_stress_entries_user_date ON body_stress_entries (user_id, date DESC, id DESC);
CREATE INDEX body_stress_entries_activity ON body_stress_entries (activity_id) WHERE activity_id IS NOT NULL;

UPDATE health_status SET zones = COALESCE((
    SELECT json_group_array(json(j.value))
    FROM json_each(health_status.zones) AS j
    WHERE json_extract(j.value, '$.zone') IN ('knee_pfps', 'achilles', 'plantar_fascia', 'shin_splints', 'iliotibial_band', 'hamstrings_calves')
), '[]')
WHERE json_valid(zones);

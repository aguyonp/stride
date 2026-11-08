-- §15.2: UI, report and prompt language; NULL follows the browser (Accept-Language).
ALTER TABLE athlete_profiles ADD COLUMN language TEXT CHECK (language IS NULL OR language IN ('fr', 'en'));

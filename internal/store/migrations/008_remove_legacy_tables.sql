-- Migration 005 has already imported recoverable VK tracks and downloads.
-- Remove the unused legacy schema without deleting files on disk.
DROP TABLE downloads;
DROP TABLE tracks;
DROP TABLE credentials;

-- Credentials are short-lived; reconnect sources after changing the format.
ALTER TABLE user_credentials RENAME TO users_credentials;
DROP TABLE users_credentials;
CREATE TABLE users_credentials (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (length(source) > 0),
    data BLOB NOT NULL,
    PRIMARY KEY (user_id, source)
);

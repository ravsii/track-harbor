CREATE TABLE download_control (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    stopped INTEGER NOT NULL DEFAULT 0 CHECK (stopped IN (0, 1))
);

INSERT INTO download_control (id, stopped) VALUES (1, 0);

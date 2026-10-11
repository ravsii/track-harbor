CREATE TABLE download_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    strategy TEXT NOT NULL,
    personal_proxy TEXT NOT NULL,
    public_proxy TEXT NOT NULL
);

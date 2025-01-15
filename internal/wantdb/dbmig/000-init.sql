
CREATE TABLE blobs (
    id BLOB NOT NULL PRIMARY KEY,
    data BLOB NOT NULL,
    rc INT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE stores (
    id INTEGER PRIMARY KEY AUTOINCREMENT
) STRICT;

CREATE TABLE store_blobs (
    store_id INT NOT NULL REFERENCES stores(id),
    blob_id BLOB NOT NULL REFERENCES blobs(id),
    PRIMARY KEY (store_id, blob_id)

) STRICT, WITHOUT ROWID;

CREATE TABLE sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    store_id INT NOT NULL REFERENCES stores(id),
    root TEXT NOT NULL,
    repo_dir TEXT,
    created_at BLOB NOT NULL
) STRICT;

CREATE TABLE ops (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
) STRICT;

CREATE TABLE tasks (
    id BLOB NOT NULL PRIMARY KEY,

    op INT NOT NULL REFERENCES ops(id),
    input BLOB NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE jobs (
    rowid INTEGER PRIMARY KEY AUTOINCREMENT,

    task BLOB NOT NULL REFERENCES tasks(id),
    next_idx INT NOT NULL DEFAULT 0,
    state INT NOT NULL DEFAULT 1,
    res_data BLOB,
    errcode INT
) STRICT;

CREATE TABLE job_roots (
    idx INTEGER PRIMARY KEY AUTOINCREMENT,
    job_row INT REFERENCES jobs(id),
    store_id INT REFERENCES stores(id)
) STRICT;

CREATE TABLE job_children (
    parent INT NOT NULL REFERENCES jobs(rowid),
    idx INT NOT NULL,
    child INT NOT NULL REFERENCES jobs(rowid),
    PRIMARY KEY(parent, idx, child)
);

CREATE INDEX idx_job_cache ON jobs (task) WHERE "state" = 3 AND errcode = 0;
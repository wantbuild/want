
CREATE TABLE blobs (
    id BLOB NOT NULL,
    data BLOB NOT NULL,
    rc INT NOT NULL,

    PRIMARY KEY(id)
) STRICT, WITHOUT ROWID;

CREATE TABLE stores (
    id INTEGER PRIMARY KEY AUTOINCREMENT
) STRICT;

CREATE TABLE store_blobs (
    store_id INT NOT NULL REFERENCES stores(id),
    blob_id BLOB NOT NULL REFERENCES blobs(id),

    PRIMARY KEY (store_id, blob_id)
) STRICT;


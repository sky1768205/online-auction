CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    display_name  TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE categories (
    id   BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE
);

-- Все денежные суммы хранятся в копейках (BIGINT), чтобы не было ошибок округления.
CREATE TABLE lots (
    id            BIGSERIAL PRIMARY KEY,
    seller_id     BIGINT      NOT NULL REFERENCES users (id),
    category_id   BIGINT      NOT NULL REFERENCES categories (id),
    title         TEXT        NOT NULL CHECK (length(title) BETWEEN 3 AND 200),
    description   TEXT        NOT NULL DEFAULT '',
    start_price   BIGINT      NOT NULL CHECK (start_price > 0),
    min_step      BIGINT      NOT NULL CHECK (min_step > 0),

    -- Денормализованное состояние торгов: обновляется под блокировкой строки
    -- вместе со вставкой ставки, поэтому всегда согласовано с таблицей bids.
    current_price BIGINT,
    leader_id     BIGINT REFERENCES users (id),
    bid_count     INT         NOT NULL DEFAULT 0,

    starts_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    ends_at       TIMESTAMPTZ NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'finished', 'cancelled')),

    -- Результат торгов фиксируется при закрытии.
    winner_id     BIGINT REFERENCES users (id),
    final_price   BIGINT,
    closed_at     TIMESTAMPTZ,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    CHECK (current_price IS NULL OR current_price >= start_price)
);

-- Фоновое закрытие торгов ищет только активные лоты с истёкшим таймером.
CREATE INDEX lots_active_ends_at_idx ON lots (ends_at) WHERE status = 'active';
CREATE INDEX lots_category_status_idx ON lots (category_id, status);

CREATE TABLE lot_images (
    id       BIGSERIAL PRIMARY KEY,
    lot_id   BIGINT NOT NULL REFERENCES lots (id) ON DELETE CASCADE,
    url      TEXT   NOT NULL,
    position INT    NOT NULL DEFAULT 0
);

CREATE INDEX lot_images_lot_idx ON lot_images (lot_id, position);

CREATE TABLE bids (
    id         BIGSERIAL PRIMARY KEY,
    lot_id     BIGINT      NOT NULL REFERENCES lots (id) ON DELETE CASCADE,
    user_id    BIGINT      NOT NULL REFERENCES users (id),
    amount     BIGINT      NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX bids_lot_idx ON bids (lot_id, id DESC);
CREATE INDEX bids_user_idx ON bids (user_id);

INSERT INTO categories (name, slug) VALUES
    ('Электроника', 'electronics'),
    ('Искусство', 'art'),
    ('Коллекционирование', 'collectibles'),
    ('Часы и украшения', 'watches'),
    ('Книги', 'books'),
    ('Дом и интерьер', 'home');

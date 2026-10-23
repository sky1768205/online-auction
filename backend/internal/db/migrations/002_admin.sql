-- Роли и модерация: администраторы, блокировка пользователей, снятие лотов с торгов.
ALTER TABLE users
    ADD COLUMN role       TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    ADD COLUMN blocked_at TIMESTAMPTZ;

-- Причина снятия лота с торгов (status = 'cancelled').
ALTER TABLE lots ADD COLUMN cancel_reason TEXT;

-- Лента последних ставок и статистика за сутки в админ-панели.
CREATE INDEX bids_created_idx ON bids (created_at DESC);

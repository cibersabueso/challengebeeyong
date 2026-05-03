-- Seed inventory aligned with the reference mockup of the challenge.
-- Idempotent: only inserts when the items table is empty.
INSERT INTO items (name, total, reserved)
SELECT 'Vintage Camera', 20, 6
WHERE NOT EXISTS (SELECT 1 FROM items);

INSERT INTO items (name, total, reserved)
SELECT 'Mechanical Watch', 10, 6
WHERE (SELECT COUNT(*) FROM items) < 6;

INSERT INTO items (name, total, reserved)
SELECT 'Acoustic Guitar', 16, 8
WHERE (SELECT COUNT(*) FROM items) < 6;

INSERT INTO items (name, total, reserved)
SELECT 'Smart Flask', 20, 1
WHERE (SELECT COUNT(*) FROM items) < 6;

INSERT INTO items (name, total, reserved)
SELECT 'Running Shoes', 12, 12
WHERE (SELECT COUNT(*) FROM items) < 6;

INSERT INTO items (name, total, reserved)
SELECT 'Gaming Mouse', 15, 14
WHERE (SELECT COUNT(*) FROM items) < 6;

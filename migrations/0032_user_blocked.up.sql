ALTER TABLE users
    ADD COLUMN IF NOT EXISTS blocked BOOLEAN NOT NULL DEFAULT false;

UPDATE users
   SET blocked = true
 WHERE blocked = false
   AND (
        lower(split_part(email, '@', 2)) = 'shit.ralsei.lol'
        OR lower(split_part(email, '@', 2)) LIKE '%.shit.ralsei.lol'
   );

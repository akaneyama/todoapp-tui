-- Runs once on first start of the MariaDB container.
CREATE DATABASE IF NOT EXISTS todoapp_test CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
GRANT ALL PRIVILEGES ON todoapp_test.* TO 'todoapp'@'%';
FLUSH PRIVILEGES;

-- +goose Up
-- Partition count and encoding are durable contracts. Never repartition this topic.
INSERT INTO event_topic_partition (topic, partition)
SELECT 'authorization.transactions.v1', generate_series(0, 31);
-- +goose Down
DELETE FROM event_consumer_offset WHERE topic = 'authorization.transactions.v1';
DELETE FROM event_log WHERE topic = 'authorization.transactions.v1';
DELETE FROM event_topic_partition WHERE topic = 'authorization.transactions.v1';

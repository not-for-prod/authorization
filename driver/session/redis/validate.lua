local now = redis.call('TIME')
local nowms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
if redis.call('EXISTS', KEYS[1]) == 0 then return '' end
if redis.call('HGET', KEYS[1], 'revoked') ~= '0' then return '' end
if redis.call('HGET', KEYS[1], 'digest') ~= ARGV[1] then return '' end
if nowms >= tonumber(redis.call('HGET', KEYS[1], 'expires')) then return '' end
return redis.call('HGET', KEYS[1], 'session')

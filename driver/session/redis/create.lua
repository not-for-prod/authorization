-- KEYS share a tenant hash tag. Never change the stored issuance on a retry.
local now = redis.call('TIME')
local nowms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
if nowms >= tonumber(ARGV[2]) then return {'expired', ''} end
if redis.call('EXISTS', KEYS[1]) == 1 then
    if redis.call('HGET', KEYS[1], 'principal') ~= ARGV[1] then
        return {'conflict', ''}
    end
    if nowms >= tonumber(redis.call('HGET', KEYS[1], 'deadline')) then return {'expired', ''} end
    return {'ok', redis.call('HGET', KEYS[1], 'issued')}
end
if nowms >= tonumber(ARGV[3]) then return {'expired', ''} end
if redis.call('EXISTS', KEYS[2]) == 1 then return {'collision', ''} end
redis.call('HSET', KEYS[2], 'session', ARGV[5], 'digest', ARGV[6], 'expires', ARGV[3], 'revoked', '0')
redis.call('HSET', KEYS[1], 'principal', ARGV[1], 'deadline', ARGV[2], 'issued', ARGV[4])
local retention = math.max(tonumber(ARGV[2]), tonumber(ARGV[3]))
redis.call('PEXPIREAT', KEYS[2], retention)
redis.call('PEXPIREAT', KEYS[1], retention)
return {'ok', ARGV[4]}

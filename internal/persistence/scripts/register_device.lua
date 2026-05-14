local key = KEYS[1]
local device = ARGV[1]
local max_devices = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])

if redis.call('SISMEMBER', key, device) == 1 then
  redis.call('EXPIRE', key, ttl)
  return 1
end

local count = redis.call('SCARD', key)
if count >= max_devices then
  return 0
end

redis.call('SADD', key, device)
redis.call('EXPIRE', key, ttl)
return 1

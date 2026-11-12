package persistence

import (
	"context"
	"fmt"
	"sync"
	"time"

	cprofile "github.com/cherry-game/cherry/profile"
	"github.com/example/mmo-server/internal/gtime"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type config struct {
	mysqlDSN         string
	configDSN        string
	maxOpenConn      int
	maxIdleConn      int
	configMaxOpen    int
	configMaxIdle    int
	cacheTTL         time.Duration
	opTimeout        time.Duration
	accessTTL        time.Duration
	refreshTTL       time.Duration
	loginFailLimit   int
	loginFailWindow  time.Duration
	loginBlockTTL    time.Duration
	deviceSessionTTL time.Duration
	keyPrefix        string
	maxCharacters    int
}

// defaultMaxCharacters 是单账号最大未删除角色数的默认值。
const defaultMaxCharacters = 3

var (
	once       sync.Once
	initErr    error
	db         *gorm.DB
	cfgOnce    sync.Once
	cfgInitErr error
	cfgDB      *gorm.DB
	rdb        *redis.Client
	cfg        config
)

func Init() error {
	once.Do(func() {
		cfg = loadConfig()
		if cfg.mysqlDSN == "" {
			initErr = fmt.Errorf("mysql.dsn is empty in profile")
			return
		}

		gdb, err := openMySQL(cfg.mysqlDSN, cfg.maxOpenConn, cfg.maxIdleConn)
		if err != nil {
			initErr = err
			return
		}
		sqlDB, err := gdb.DB()
		if err != nil {
			initErr = err
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		rcfg := cprofile.GetConfig("redis")
		rdb = redis.NewClient(&redis.Options{
			Addr:         rcfg.GetString("address", "127.0.0.1:6379"),
			Password:     rcfg.GetString("password", ""),
			DB:           rcfg.GetInt("db", 0),
			PoolSize:     rcfg.GetInt("pool_size", 16),
			DialTimeout:  time.Duration(rcfg.GetInt("dial_timeout", 3)) * time.Second,
			ReadTimeout:  time.Duration(rcfg.GetInt("read_timeout", 2)) * time.Second,
			WriteTimeout: time.Duration(rcfg.GetInt("write_timeout", 2)) * time.Second,
		})
		if err = rdb.Ping(ctx).Err(); err != nil {
			_ = rdb.Close()
			rdb = nil
			_ = sqlDB.Close()
			initErr = err
			return
		}

		if err = autoMigrateModels(gdb.WithContext(ctx)); err != nil {
			_ = rdb.Close()
			rdb = nil
			_ = sqlDB.Close()
			initErr = err
			return
		}
		gtime.LoadBiasFromRedis(ctx, rdb, cfg.keyPrefix)
		db = gdb
	})
	return initErr
}

func loadConfig() config {
	my := cprofile.GetConfig("mysql")
	rd := cprofile.GetConfig("redis")
	player := cprofile.GetConfig("player")
	maxCharacters := player.GetInt("max_characters", 3)
	if maxCharacters < 1 {
		maxCharacters = 1
	}
	configOpen := my.GetInt("config_max_open_conns", 4)
	if configOpen < 1 {
		configOpen = 4
	}
	configIdle := my.GetInt("config_max_idle_conns", 2)
	if configIdle < 1 || configIdle > configOpen {
		configIdle = configOpen
	}
	return config{
		mysqlDSN:         my.GetString("dsn", ""),
		configDSN:        my.GetString("config_dsn", ""),
		maxOpenConn:      my.GetInt("max_open_conns", 20),
		maxIdleConn:      my.GetInt("max_idle_conns", 10),
		configMaxOpen:    configOpen,
		configMaxIdle:    configIdle,
		cacheTTL:         time.Duration(rd.GetInt("cache_ttl_seconds", 600)) * time.Second,
		opTimeout:        time.Duration(rd.GetInt("op_timeout_seconds", 2)) * time.Second,
		accessTTL:        time.Duration(rd.GetInt("access_ttl_seconds", 900)) * time.Second,
		refreshTTL:       time.Duration(rd.GetInt("refresh_ttl_seconds", rd.GetInt("token_ttl_seconds", 86400))) * time.Second,
		loginFailLimit:   rd.GetInt("login_fail_limit", 5),
		loginFailWindow:  time.Duration(rd.GetInt("login_fail_window_seconds", 300)) * time.Second,
		loginBlockTTL:    time.Duration(rd.GetInt("login_block_seconds", 600)) * time.Second,
		deviceSessionTTL: time.Duration(rd.GetInt("device_session_ttl_seconds", 7200)) * time.Second,
		keyPrefix:        rd.GetString("key_prefix", "mmo"),
		maxCharacters:    maxCharacters,
	}
}

func CacheTTL() time.Duration {
	return cfg.cacheTTL
}

func KeyPrefix() string {
	return cfg.keyPrefix
}

func AccessTTL() time.Duration {
	return cfg.accessTTL
}

func RefreshTTL() time.Duration {
	return cfg.refreshTTL
}

func LoginFailLimit() int {
	return cfg.loginFailLimit
}

func LoginFailWindow() time.Duration {
	return cfg.loginFailWindow
}

func LoginBlockTTL() time.Duration {
	return cfg.loginBlockTTL
}

func DeviceSessionTTL() time.Duration {
	return cfg.deviceSessionTTL
}

// MaxCharacters 返回单账号可持有的最大未删除角色数，默认 3，最少 1。
// 未通过 Init 加载配置时（如单测）返回默认值，避免零值导致上限误拦截。
func MaxCharacters() int {
	if cfg.maxCharacters < 1 {
		return defaultMaxCharacters
	}
	return cfg.maxCharacters
}

// DB 返回玩家库 GORM 实例（会先 Init）。
func DB() (*gorm.DB, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	return db, nil
}

// ConfigDB 返回配置库连接，并在首次调用时迁移 cfg_*。
// 登录节点不要调用。单测里两个句柄指向同一个 sqlite。
func ConfigDB() (*gorm.DB, error) {
	cfgOnce.Do(func() {
		if cfgDB != nil {
			return
		}
		dsn := cfg.configDSN
		maxOpen := cfg.configMaxOpen
		maxIdle := cfg.configMaxIdle
		if dsn == "" {
			my := cprofile.GetConfig("mysql")
			dsn = my.GetString("config_dsn", "")
			maxOpen = my.GetInt("config_max_open_conns", 4)
			maxIdle = my.GetInt("config_max_idle_conns", 2)
		}
		if dsn == "" {
			cfgInitErr = fmt.Errorf("mysql.config_dsn is empty in profile")
			return
		}
		if maxOpen < 1 {
			maxOpen = 4
		}
		if maxIdle < 1 || maxIdle > maxOpen {
			maxIdle = maxOpen
		}
		gdb, err := openMySQL(dsn, maxOpen, maxIdle)
		if err != nil {
			cfgInitErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err = autoMigrateConfig(gdb.WithContext(ctx)); err != nil {
			if sqlDB, dbErr := gdb.DB(); dbErr == nil {
				_ = sqlDB.Close()
			}
			cfgInitErr = err
			return
		}
		cfgDB = gdb
	})
	if cfgInitErr != nil {
		return nil, cfgInitErr
	}
	if cfgDB == nil {
		return nil, fmt.Errorf("config db unavailable")
	}
	return cfgDB, nil
}

// openMySQL 打开一个 MySQL 连接并套上连接池上限。
func openMySQL(dsn string, maxOpen, maxIdle int) (*gorm.DB, error) {
	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return gdb, nil
}

// SaveTimeBias 将游戏时间偏置写入 Redis 并更新本进程；Redis 不可用则失败。
func SaveTimeBias(ctx context.Context, sec int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if sec < 0 {
		sec = 0
	}
	if rdb == nil {
		return fmt.Errorf("redis unavailable")
	}
	return gtime.SaveBiasToRedis(ctx, rdb, KeyPrefix(), sec)
}

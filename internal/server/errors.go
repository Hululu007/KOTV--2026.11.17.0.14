package server

import "errors"

// ErrNeedAdmin 远端普通用户试图新增共享仓/直播源、清共享缓存或读写主机路径备份等。
var ErrNeedAdmin = errors.New("需要管理员")

# Log Backup CR 与内核状态同步机制分析

## 一、背景与问题

在此次改动之前，Log Backup的CR（Custom Resource）状态与实际在TiKV/PD etcd中的内核状态可能存在不一致的情况。这些改动（从commit 1b2fab09到当前HEAD）引入了一个状态同步机制，确保CR能够正确反映log backup在内核中的实际状态。

## 二、核心设计思路

### 2.1 设计目标

1. **状态一致性**：确保Log Backup CR的状态与etcd中的实际状态保持一致
2. **自动修正**：当检测到状态不一致时，自动同步并修正CR状态
3. **错误处理**：正确解析和展示内核中的错误信息
4. **向后兼容**：支持Pause V1和V2两种格式

### 2.2 状态同步时机

状态同步发生在`skipBackupSync`函数中：

```go
func (bm *backupManager) skipBackupSync(backup *v1alpha1.Backup) (bool, error) {
    switch backup.Spec.Mode {
    case v1alpha1.BackupModeLog:
        canSkip, err := bm.skipLogBackupSync(backup)
        if err != nil {
            return false, errors.Trace(err)
        }
        if !canSkip {
            // The log backup command needs to be executed, don't skip
            return false, nil
        }
        // The log backup command can be skipped, but we still need to sync kernel status
        return bm.SyncLogKernelStatus(backup)
    // ...
    }
}
```

关键点：
- 即使log backup命令可以跳过（已经执行过），仍然需要同步内核状态
- 这确保了CR始终反映最新的内核状态

## 三、etcd 键路径结构

Log Backup在etcd中的数据结构：

```
/tidb/br-stream/
├── /info/{task-name}        # 任务基本信息
├── /checkpoint/{task-name}   # 检查点信息
├── /pause/{task-name}        # 暂停状态信息
└── /last-error/{task-name}   # 最后的错误信息（Pause V1使用）
```

常量定义：
```go
const (
    streamKeyPrefix    = "/tidb/br-stream"
    taskInfoPath      = "/info"
    taskCheckpointPath = "/checkpoint"
    taskLastErrorPath = "/last-error"
    taskPausePath     = "/pause"
)
```

## 四、状态同步实现

### 4.1 主要同步函数 - SyncLogKernelStatus

```go
func (bm *backupManager) SyncLogKernelStatus(backup *v1alpha1.Backup) (bool, error) {
    // 1. 如果状态已经是Stopped，直接返回
    if backup.Status.Phase == v1alpha1.BackupStopped {
        return true, nil
    }
    
    // 2. 获取etcd客户端
    etcdCli, err := bm.deps.PDControl.GetPDEtcdClient(...)
    
    // 3. 检查log backup key是否存在
    exist, err := bm.checkLogKeyExist(etcdCli, ns, name)
    if !exist {
        // Stop命令特殊处理：key不存在是预期的
        if backup.Spec.LogSubcommand == v1alpha1.LogStopCommand {
            return true, nil
        }
        // 其他命令，key不存在是错误
        bm.statusUpdater.Update(backup, &v1alpha1.BackupCondition{
            Type:    v1alpha1.BackupFailed,
            Reason:  "LogBackupKeyNotFound",
            Message: "log backup etcd key not found in routine sync",
        }, updateStatus)
        return false, fmt.Errorf("%s log backup etcd key not found", logPrefix)
    }
    
    // 4. 解析暂停状态
    pauseStatus, err := bm.parsePauseStatus(etcdCli, ns, name)
    
    // 5. 获取内核状态并检查一致性
    kernelState := getLogBackupKernelState(pauseStatus.IsPaused)
    expectedCommand := backup.Spec.LogSubcommand
    
    // 6. 如果状态不一致，同步更新
    if !isCommandConsistentWithKernelState(expectedCommand, kernelState) {
        actualCommand := getCommandForKernelState(kernelState)
        
        bm.statusUpdater.Update(backup, &v1alpha1.BackupCondition{
            Command: actualCommand,
            Type:    v1alpha1.BackupComplete,
            Status:  corev1.ConditionTrue,
            Reason:  "LogBackupKernelSync",
            Message: fmt.Sprintf("Synced with kernel state: %s. %s", 
                                kernelState, pauseStatus.Message),
        }, updateStatus)
    }
    
    return true, nil
}
```

### 4.2 暂停状态解析

系统支持两种暂停格式：

#### Pause V1（旧格式）
- pause key存在但值为空
- 错误信息存储在`/last-error`路径下

```go
func (bm *backupManager) handlePauseV1(etcdCli pdapi.PDEtcdClient, ns, name string) (PauseStatus, error) {
    status := PauseStatus{IsPaused: true}
    
    // 从 /last-error 路径读取错误信息
    errorKey := path.Join(streamKeyPrefix, taskLastErrorPath, name)
    errorKVs, err := bm.queryEtcdKey(etcdCli, errorKey, false)
    
    if len(errorKVs) > 0 {
        errMsg, err := ParseBackupError(errorKVs[0].Value)
        status.Message = errMsg
    }
    
    return status, nil
}
```

#### Pause V2（新格式）
- pause key的值包含详细的暂停信息
- 支持severity级别（ERROR/MANUAL）
- 包含操作者信息、时间戳等元数据

```go
type PauseV2Info struct {
    Severity         string    `json:"severity"`          // ERROR或MANUAL
    OperatorHostName string    `json:"operation_hostname"`
    OperatorPID      int       `json:"operation_pid"`
    OperationTime    time.Time `json:"operation_time"`
    PayloadType      string    `json:"payload_type"`      // MIME类型
    Payload          []byte    `json:"payload"`           // 实际的错误信息
}
```

### 4.3 内核状态定义

```go
type LogBackupKernelState string

const (
    LogBackupKernelRunning LogBackupKernelState = "running"
    LogBackupKernelPaused  LogBackupKernelState = "paused"
)
```

## 五、状态映射关系

### 5.1 命令与内核状态的一致性检查

```go
func isCommandConsistentWithKernelState(command LogSubCommandType, kernelState LogBackupKernelState) bool {
    switch kernelState {
    case LogBackupKernelRunning:
        // Running状态与Start或Resume命令一致
        return command == LogStartCommand || command == LogResumeCommand
    case LogBackupKernelPaused:
        // Paused状态与Pause命令一致
        return command == LogPauseCommand
    default:
        return false
    }
}
```

### 5.2 状态映射表

| 内核状态 | 一致的CR命令 | 不一致时的修正命令 |
|---------|-------------|------------------|
| Running | log-start, log-resume | log-resume |
| Paused | log-pause | log-pause |
| Key不存在 | log-stop | - |

### 5.3 特殊情况处理

1. **Stop命令的特殊处理**：
   - 当执行stop命令后，etcd中的key会被删除
   - 因此key不存在对于stop命令是正常的
   - 其他命令如果key不存在则是错误状态

2. **状态不一致的自动修正**：
   - 例如：CR显示`log-start`，但内核状态是`paused`
   - 系统会自动将CR更新为`log-pause`，并记录同步事件

## 六、新增的数据结构

### 6.1 PauseStatus
```go
type PauseStatus struct {
    IsPaused bool    // 是否处于暂停状态
    Message  string  // 暂停原因或错误信息
}
```

### 6.2 StreamBackupError
```go
type StreamBackupError struct {
    HappenAt     uint64 `json:"happen_at"`      // 错误发生时间（Unix时间戳）
    ErrorCode    string `json:"error_code"`     // 错误代码
    ErrorMessage string `json:"error_message"`  // 错误消息
    StoreId      uint64 `json:"store_id"`       // 发生错误的store ID
}
```

### 6.3 TimeSynced字段
在BackupStatus中新增了`TimeSynced`字段，记录最后一次同步内核状态的时间：
```go
type BackupStatus struct {
    // ...
    TimeSynced *metav1.Time `json:"timeSynced,omitempty"`
    // ...
}
```

## 七、错误处理机制

### 7.1 Payload类型处理

系统支持两种payload类型：
1. **text/plain**：直接使用字符串作为错误信息
2. **application/x-protobuf?messagetype=brpb.StreamBackupError**：解析为StreamBackupError结构

```go
func (p *PauseV2Info) ParseError() (string, error) {
    m, param, err := mime.ParseMediaType(p.PayloadType)
    
    switch m {
    case "text/plain":
        return string(p.Payload), nil
    case "application/x-protobuf":
        msgType := param["messagetype"]
        if msgType == "brpb.StreamBackupError" {
            return ParseBackupError(p.Payload)
        }
    }
}
```

### 7.2 错误重试机制

使用`retry.OnError`进行重试，确保etcd查询的可靠性：
```go
func (bm *backupManager) checkLogKeyExist(etcdCli pdapi.PDEtcdClient, ns, name string) (bool, error) {
    var exists bool
    checkLogExist := func() error {
        kvs, err := bm.queryEtcdKey(etcdCli, logKey, false)
        if err != nil {
            return fmt.Errorf("query etcd key %s failed: %v", logKey, err)
        }
        exists = (len(kvs) > 0)
        return nil
    }
    return exists, retry.OnError(retry.DefaultRetry, 
                                func(e error) bool { return e != nil }, 
                                checkLogExist)
}
```

## 八、总结

这套状态同步机制的核心价值：

1. **保证一致性**：CR状态始终反映内核的真实状态
2. **自动修复**：检测到不一致时自动修正，无需人工干预
3. **完整的错误信息**：保留并展示内核中的详细错误信息
4. **向后兼容**：同时支持新旧两种暂停格式

通过这种设计，即使在以下场景中也能保证状态正确：
- 外部工具直接修改了etcd中的log backup状态
- 网络问题导致命令执行结果未能及时反映到CR
- TiKV节点故障导致log backup自动暂停

这是一个典型的"最终一致性"设计，通过定期同步确保系统状态的准确性。
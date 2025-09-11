# Log Backup 重试功能 - 代码实现示例

## 一、核心数据结构修改

### 1.1 types.go 修改

```go
// pkg/apis/pingcap/v1alpha1/types.go

// BackupSpec 添加LogBackupRetryPolicy字段
type BackupSpec struct {
    // ... 现有字段 ...
    
    // LogBackupRetryPolicy log backup的重试策略
    // +optional
    LogBackupRetryPolicy *LogBackupRetryPolicy `json:"logBackupRetryPolicy,omitempty"`
}

// LogBackupRetryPolicy 定义log backup的重试策略
type LogBackupRetryPolicy struct {
    // Enabled 是否启用重试，默认false
    // +kubebuilder:default=false
    Enabled bool `json:"enabled,omitempty"`
    
    // MaxRetryTimes 最大重试次数
    // +kubebuilder:default=3
    MaxRetryTimes int `json:"maxRetryTimes,omitempty"`
    
    // InitialInterval 初始重试间隔
    // +kubebuilder:default="30s"
    InitialInterval string `json:"initialInterval,omitempty"`
    
    // MaxInterval 最大重试间隔
    // +kubebuilder:default="5m"
    MaxInterval string `json:"maxInterval,omitempty"`
    
    // Multiplier 退避系数
    // +kubebuilder:default=2.0
    Multiplier float64 `json:"multiplier,omitempty"`
    
    // RetryTimeout 重试总超时时间
    // +kubebuilder:default="30m"
    RetryTimeout string `json:"retryTimeout,omitempty"`
}

// BackupStatus 添加重试记录
type BackupStatus struct {
    // ... 现有字段 ...
    
    // LogBackupRetryRecords log backup的重试记录
    LogBackupRetryRecords []LogBackupRetryRecord `json:"logBackupRetryRecords,omitempty"`
    
    // LastKernelState 上次检查的内核状态
    LastKernelState string `json:"lastKernelState,omitempty"`
    
    // LastKernelStateTime 上次检查内核状态的时间
    LastKernelStateTime *metav1.Time `json:"lastKernelStateTime,omitempty"`
}

// LogBackupRetryRecord 重试记录
type LogBackupRetryRecord struct {
    // RetryNum 重试序号
    RetryNum int `json:"retryNum"`
    
    // Command 重试的命令
    Command LogSubCommandType `json:"command"`
    
    // FailedAt 失败时间
    FailedAt metav1.Time `json:"failedAt"`
    
    // FailureReason 失败原因
    FailureReason string `json:"failureReason"`
    
    // OriginalError 原始错误信息
    OriginalError string `json:"originalError,omitempty"`
    
    // RetryAt 重试时间
    RetryAt *metav1.Time `json:"retryAt,omitempty"`
    
    // Success 重试是否成功
    Success bool `json:"success,omitempty"`
}
```

## 二、幂等性检查增强

### 2.1 改进的 skipLogBackupSync 函数

```go
// pkg/backup/backup/backup_manager.go

func (bm *backupManager) skipLogBackupSync(backup *v1alpha1.Backup) (bool, error) {
    if backup.Spec.Mode != v1alpha1.BackupModeLog {
        return false, nil
    }
    
    ns, name := backup.Namespace, backup.Name
    command := v1alpha1.ParseLogBackupSubcommand(backup)
    
    // 1. 获取内核状态
    kernelState, err := bm.getLogBackupKernelState(backup)
    if err != nil {
        klog.Errorf("Failed to get kernel state for %s/%s: %v", ns, name, err)
        // 如果获取失败，按原有逻辑处理
        return bm.skipLogBackupSyncLegacy(backup)
    }
    
    // 2. 基于内核状态的幂等性检查
    shouldSkip, needSync := bm.checkIdempotency(backup, command, kernelState)
    
    // 3. 记录内核状态到CR
    if err := bm.updateKernelStateInCR(backup, kernelState); err != nil {
        klog.Errorf("Failed to update kernel state in CR %s/%s: %v", ns, name, err)
    }
    
    // 4. 如果需要同步状态
    if needSync {
        return bm.SyncLogKernelStatus(backup)
    }
    
    return shouldSkip, nil
}

// 获取log backup的内核状态
func (bm *backupManager) getLogBackupKernelState(backup *v1alpha1.Backup) (LogBackupKernelState, error) {
    tc, err := bm.backupTracker.GetLogBackupTC(backup)
    if err != nil {
        return LogBackupKernelUnknown, err
    }
    
    etcdCli, err := bm.deps.PDControl.GetPDEtcdClient(
        pdapi.Namespace(tc.Namespace),
        tc.Name,
        tc.IsTLSClusterEnabled(),
        pdapi.ClusterRef(tc.Spec.ClusterDomain),
    )
    if err != nil {
        return LogBackupKernelUnknown, err
    }
    defer etcdCli.Close()
    
    // 检查key是否存在
    exist, err := bm.checkLogKeyExist(etcdCli, backup.Namespace, backup.Name)
    if err != nil {
        return LogBackupKernelUnknown, err
    }
    if !exist {
        return LogBackupKernelNotExist, nil
    }
    
    // 检查暂停状态
    pauseStatus, err := bm.parsePauseStatus(etcdCli, backup.Namespace, backup.Name)
    if err != nil {
        return LogBackupKernelUnknown, err
    }
    
    if pauseStatus.IsPaused {
        return LogBackupKernelPaused, nil
    }
    
    return LogBackupKernelRunning, nil
}

// 幂等性检查
func (bm *backupManager) checkIdempotency(
    backup *v1alpha1.Backup, 
    command v1alpha1.LogSubCommandType,
    kernelState LogBackupKernelState,
) (shouldSkip bool, needSync bool) {
    
    switch command {
    case v1alpha1.LogStartCommand:
        switch kernelState {
        case LogBackupKernelRunning:
            // 已经在运行，幂等
            return true, false
        case LogBackupKernelPaused:
            // 需要resume而不是start
            return false, true
        case LogBackupKernelNotExist:
            // 需要执行start
            return false, false
        }
        
    case v1alpha1.LogStopCommand:
        if kernelState == LogBackupKernelNotExist {
            // 已经停止，幂等
            return true, false
        }
        // 需要执行stop
        return false, false
        
    case v1alpha1.LogPauseCommand:
        switch kernelState {
        case LogBackupKernelPaused:
            // 已经暂停，幂等
            return true, false
        case LogBackupKernelRunning:
            // 需要执行pause
            return false, false
        case LogBackupKernelNotExist:
            // 不存在无法pause
            return false, true
        }
        
    case v1alpha1.LogResumeCommand:
        switch kernelState {
        case LogBackupKernelRunning:
            // 已经在运行，幂等
            return true, false
        case LogBackupKernelPaused:
            // 需要执行resume
            return false, false
        case LogBackupKernelNotExist:
            // 不存在无法resume
            return false, true
        }
        
    case v1alpha1.LogTruncateCommand:
        if kernelState != LogBackupKernelRunning {
            // truncate需要在running状态
            return false, true
        }
        // 检查是否已经truncate到指定位置
        return bm.isAlreadyTruncated(backup), false
    }
    
    return false, false
}
```

## 三、重试逻辑实现

### 3.1 backup_controller.go 修改

```go
// pkg/controller/backup/backup_controller.go

func (c *Controller) detectBackupJobFailure(backup *v1alpha1.Backup) (
    jobFailed bool, reason string, originalReason string, err error) {
    
    ns, name := backup.GetNamespace(), backup.GetName()
    
    // Log backup的失败检测
    if backup.Spec.Mode == v1alpha1.BackupModeLog {
        return c.detectLogBackupFailure(backup)
    }
    
    // 原有的snapshot backup逻辑
    // ...
}

// 检测log backup失败
func (c *Controller) detectLogBackupFailure(backup *v1alpha1.Backup) (
    jobFailed bool, reason string, originalReason string, err error) {
    
    ns, name := backup.GetNamespace(), backup.GetName()
    jobName := backup.GetBackupJobName()
    
    // 1. 检查Job状态
    job, err := c.deps.JobLister.Jobs(ns).Get(jobName)
    if err != nil {
        if errors.IsNotFound(err) {
            // Job不存在，可能还没创建
            return false, "", "", nil
        }
        return false, "", "", err
    }
    
    // 2. 检查Job是否失败
    for _, condition := range job.Status.Conditions {
        if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
            reason = fmt.Sprintf("Job %s failed", jobName)
            originalReason = condition.Reason
            
            // 3. 分类失败原因
            if c.isRetryableFailure(originalReason) {
                return true, reason, originalReason, nil
            } else {
                // 不可重试的失败
                return true, reason, originalReason, nil
            }
        }
    }
    
    // 4. 检查内核状态异常
    if err := c.checkKernelStateAnomaly(backup); err != nil {
        return true, "KernelStateAnomaly", err.Error(), nil
    }
    
    return false, "", "", nil
}

// 判断是否可重试的失败
func (c *Controller) isRetryableFailure(reason string) bool {
    retryableReasons := []string{
        "DeadlineExceeded",
        "BackoffLimitExceeded", 
        "PodFailed",
        "Evicted",
        "NodeLost",
        "Unknown",
    }
    
    for _, r := range retryableReasons {
        if strings.Contains(reason, r) {
            return true
        }
    }
    
    return false
}

// 重试log backup
func (c *Controller) retryLogBackup(backup *v1alpha1.Backup, reason, originalReason string) error {
    ns, name := backup.GetNamespace(), backup.GetName()
    
    // 1. 检查是否启用重试
    if backup.Spec.LogBackupRetryPolicy == nil || !backup.Spec.LogBackupRetryPolicy.Enabled {
        klog.V(4).Infof("Retry not enabled for log backup %s/%s", ns, name)
        return c.markLogBackupFailed(backup, reason, originalReason)
    }
    
    // 2. 检查重试次数
    retryCount := len(backup.Status.LogBackupRetryRecords)
    maxRetries := backup.Spec.LogBackupRetryPolicy.MaxRetryTimes
    if maxRetries == 0 {
        maxRetries = 3 // 默认值
    }
    
    if retryCount >= maxRetries {
        klog.Infof("Log backup %s/%s exceeded max retry times %d", ns, name, maxRetries)
        return c.markLogBackupFailed(backup, "ExceededMaxRetries", 
            fmt.Sprintf("Failed after %d retries. Last error: %s", retryCount, originalReason))
    }
    
    // 3. 检查重试超时
    if c.isRetryTimeout(backup) {
        klog.Infof("Log backup %s/%s retry timeout", ns, name)
        return c.markLogBackupFailed(backup, "RetryTimeout", originalReason)
    }
    
    // 4. 计算重试间隔
    interval := c.calculateRetryInterval(backup, retryCount)
    
    // 5. 检查是否到了重试时间
    if !c.isTimeToRetry(backup, interval) {
        klog.V(4).Infof("Log backup %s/%s not time to retry yet", ns, name)
        c.queue.AddAfter(cache.MetaNamespaceKeyFunc(backup), interval)
        return nil
    }
    
    // 6. 记录重试
    if err := c.recordRetryAttempt(backup, reason, originalReason); err != nil {
        return err
    }
    
    // 7. 清理旧Job
    if err := c.cleanupFailedJob(backup); err != nil {
        klog.Errorf("Failed to cleanup job for %s/%s: %v", ns, name, err)
        return err
    }
    
    // 8. 重置状态准备重试
    return c.resetForRetry(backup)
}

// 计算重试间隔（指数退避）
func (c *Controller) calculateRetryInterval(backup *v1alpha1.Backup, retryCount int) time.Duration {
    policy := backup.Spec.LogBackupRetryPolicy
    
    // 解析初始间隔
    initialInterval, _ := time.ParseDuration(policy.InitialInterval)
    if initialInterval == 0 {
        initialInterval = 30 * time.Second
    }
    
    // 解析最大间隔
    maxInterval, _ := time.ParseDuration(policy.MaxInterval)
    if maxInterval == 0 {
        maxInterval = 5 * time.Minute
    }
    
    // 获取乘数
    multiplier := policy.Multiplier
    if multiplier == 0 {
        multiplier = 2.0
    }
    
    // 计算间隔
    interval := initialInterval
    for i := 0; i < retryCount; i++ {
        interval = time.Duration(float64(interval) * multiplier)
        if interval > maxInterval {
            interval = maxInterval
            break
        }
    }
    
    return interval
}

// 记录重试尝试
func (c *Controller) recordRetryAttempt(backup *v1alpha1.Backup, reason, originalReason string) error {
    retryRecord := v1alpha1.LogBackupRetryRecord{
        RetryNum:      len(backup.Status.LogBackupRetryRecords) + 1,
        Command:       v1alpha1.ParseLogBackupSubcommand(backup),
        FailedAt:      metav1.Now(),
        FailureReason: reason,
        OriginalError: originalReason,
        RetryAt:       &metav1.Time{Time: time.Now()},
    }
    
    backup.Status.LogBackupRetryRecords = append(backup.Status.LogBackupRetryRecords, retryRecord)
    
    return c.control.UpdateStatus(backup, &v1alpha1.BackupCondition{
        Type:    v1alpha1.BackupRetrying,
        Status:  corev1.ConditionTrue,
        Reason:  "RetryingLogBackup",
        Message: fmt.Sprintf("Retrying log backup (attempt %d/%d)", 
            retryRecord.RetryNum, backup.Spec.LogBackupRetryPolicy.MaxRetryTimes),
    }, nil)
}
```

## 四、状态机实现

### 4.1 命令转换逻辑

```go
// pkg/backup/backup/backup_manager.go

// 根据内核状态转换命令
func (bm *backupManager) translateCommand(
    backup *v1alpha1.Backup,
    expectedCommand v1alpha1.LogSubCommandType,
    kernelState LogBackupKernelState,
) (actualCommand v1alpha1.LogSubCommandType, err error) {
    
    switch expectedCommand {
    case v1alpha1.LogStartCommand:
        switch kernelState {
        case LogBackupKernelNotExist:
            // 正常start
            return v1alpha1.LogStartCommand, nil
        case LogBackupKernelPaused:
            // 转换为resume
            klog.Infof("Log backup %s/%s is paused, converting start to resume", 
                backup.Namespace, backup.Name)
            return v1alpha1.LogResumeCommand, nil
        case LogBackupKernelRunning:
            // 已经在运行，跳过
            return "", nil
        }
        
    case v1alpha1.LogStopCommand:
        if kernelState == LogBackupKernelNotExist {
            // 已经停止，跳过
            return "", nil
        }
        return v1alpha1.LogStopCommand, nil
        
    case v1alpha1.LogPauseCommand:
        switch kernelState {
        case LogBackupKernelRunning:
            return v1alpha1.LogPauseCommand, nil
        case LogBackupKernelPaused:
            // 已经暂停，跳过
            return "", nil
        case LogBackupKernelNotExist:
            return "", fmt.Errorf("cannot pause non-existent log backup")
        }
        
    case v1alpha1.LogResumeCommand:
        switch kernelState {
        case LogBackupKernelPaused:
            return v1alpha1.LogResumeCommand, nil
        case LogBackupKernelRunning:
            // 已经在运行，跳过
            return "", nil
        case LogBackupKernelNotExist:
            // 需要先start
            klog.Infof("Log backup %s/%s not exist, converting resume to start",
                backup.Namespace, backup.Name)
            return v1alpha1.LogStartCommand, nil
        }
    }
    
    return expectedCommand, nil
}

// 执行前的预检查
func (bm *backupManager) preflightCheck(backup *v1alpha1.Backup) error {
    command := v1alpha1.ParseLogBackupSubcommand(backup)
    
    // 1. 检查依赖条件
    if err := bm.checkCommandDependencies(backup, command); err != nil {
        return err
    }
    
    // 2. 检查资源状态
    if err := bm.checkResourceStatus(backup); err != nil {
        return err
    }
    
    // 3. 检查并发冲突
    if err := bm.checkConcurrentOperations(backup); err != nil {
        return err
    }
    
    return nil
}

// 检查命令依赖
func (bm *backupManager) checkCommandDependencies(
    backup *v1alpha1.Backup,
    command v1alpha1.LogSubCommandType,
) error {
    switch command {
    case v1alpha1.LogTruncateCommand:
        // Truncate需要log backup正在运行
        if backup.Status.CommitTs == "" {
            return fmt.Errorf("cannot truncate: log backup not started")
        }
    case v1alpha1.LogStopCommand, v1alpha1.LogPauseCommand:
        // 这些命令需要log backup已经启动
        if backup.Status.CommitTs == "" {
            // 可能需要等待start完成
            return controller.RequeueErrorf("waiting for log backup start to complete")
        }
    }
    
    return nil
}
```

## 五、监控指标实现

### 5.1 新增Metrics

```go
// pkg/metrics/metrics.go

var (
    // Log backup重试次数
    LogBackupRetryTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "tidb_operator",
            Subsystem: "log_backup",
            Name:      "retry_total",
            Help:      "Total number of log backup retries",
        },
        []string{"namespace", "name", "command", "reason"},
    )
    
    // Log backup重试成功次数
    LogBackupRetrySuccess = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "tidb_operator",
            Subsystem: "log_backup",
            Name:      "retry_success_total",
            Help:      "Total number of successful log backup retries",
        },
        []string{"namespace", "name", "command"},
    )
    
    // 幂等操作跳过次数
    LogBackupIdempotentSkipped = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "tidb_operator",
            Subsystem: "log_backup",
            Name:      "idempotent_skipped_total",
            Help:      "Total number of skipped idempotent operations",
        },
        []string{"namespace", "name", "command"},
    )
    
    // 内核状态同步次数
    LogBackupKernelSyncTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "tidb_operator",
            Subsystem: "log_backup",
            Name:      "kernel_sync_total",
            Help:      "Total number of kernel state syncs",
        },
        []string{"namespace", "name", "from_state", "to_state"},
    )
    
    // 重试延迟直方图
    LogBackupRetryDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Namespace: "tidb_operator",
            Subsystem: "log_backup",
            Name:      "retry_duration_seconds",
            Help:      "Duration of log backup retry attempts",
            Buckets:   prometheus.ExponentialBuckets(1, 2, 10),
        },
        []string{"namespace", "name", "command"},
    )
)

func init() {
    prometheus.MustRegister(
        LogBackupRetryTotal,
        LogBackupRetrySuccess,
        LogBackupIdempotentSkipped,
        LogBackupKernelSyncTotal,
        LogBackupRetryDuration,
    )
}
```

## 六、单元测试示例

### 6.1 幂等性测试

```go
// pkg/backup/backup/backup_manager_test.go

func TestLogBackupIdempotency(t *testing.T) {
    tests := []struct {
        name          string
        command       v1alpha1.LogSubCommandType
        kernelState   LogBackupKernelState
        expectSkip    bool
        expectSync    bool
    }{
        {
            name:        "start when already running",
            command:     v1alpha1.LogStartCommand,
            kernelState: LogBackupKernelRunning,
            expectSkip:  true,
            expectSync:  false,
        },
        {
            name:        "start when paused",
            command:     v1alpha1.LogStartCommand,
            kernelState: LogBackupKernelPaused,
            expectSkip:  false,
            expectSync:  true,
        },
        {
            name:        "stop when not exist",
            command:     v1alpha1.LogStopCommand,
            kernelState: LogBackupKernelNotExist,
            expectSkip:  true,
            expectSync:  false,
        },
        {
            name:        "pause when already paused",
            command:     v1alpha1.LogPauseCommand,
            kernelState: LogBackupKernelPaused,
            expectSkip:  true,
            expectSync:  false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup
            bm := &backupManager{}
            backup := &v1alpha1.Backup{
                Spec: v1alpha1.BackupSpec{
                    Mode: v1alpha1.BackupModeLog,
                },
            }
            
            // Test
            shouldSkip, needSync := bm.checkIdempotency(backup, tt.command, tt.kernelState)
            
            // Assert
            assert.Equal(t, tt.expectSkip, shouldSkip)
            assert.Equal(t, tt.expectSync, needSync)
        })
    }
}
```

### 6.2 重试逻辑测试

```go
func TestLogBackupRetry(t *testing.T) {
    tests := []struct {
        name           string
        retryPolicy    *v1alpha1.LogBackupRetryPolicy
        retryRecords   []v1alpha1.LogBackupRetryRecord
        failureReason  string
        expectRetry    bool
        expectInterval time.Duration
    }{
        {
            name: "first retry",
            retryPolicy: &v1alpha1.LogBackupRetryPolicy{
                Enabled:         true,
                MaxRetryTimes:   3,
                InitialInterval: "30s",
                Multiplier:      2.0,
            },
            retryRecords:   []v1alpha1.LogBackupRetryRecord{},
            failureReason:  "PodFailed",
            expectRetry:    true,
            expectInterval: 30 * time.Second,
        },
        {
            name: "second retry with backoff",
            retryPolicy: &v1alpha1.LogBackupRetryPolicy{
                Enabled:         true,
                MaxRetryTimes:   3,
                InitialInterval: "30s",
                Multiplier:      2.0,
            },
            retryRecords: []v1alpha1.LogBackupRetryRecord{
                {RetryNum: 1},
            },
            failureReason:  "PodFailed",
            expectRetry:    true,
            expectInterval: 60 * time.Second,
        },
        {
            name: "exceed max retries",
            retryPolicy: &v1alpha1.LogBackupRetryPolicy{
                Enabled:       true,
                MaxRetryTimes: 2,
            },
            retryRecords: []v1alpha1.LogBackupRetryRecord{
                {RetryNum: 1},
                {RetryNum: 2},
            },
            failureReason: "PodFailed",
            expectRetry:   false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup
            c := &Controller{}
            backup := &v1alpha1.Backup{
                Spec: v1alpha1.BackupSpec{
                    LogBackupRetryPolicy: tt.retryPolicy,
                },
                Status: v1alpha1.BackupStatus{
                    LogBackupRetryRecords: tt.retryRecords,
                },
            }
            
            // Test
            shouldRetry := c.shouldRetryLogBackup(backup, tt.failureReason)
            interval := c.calculateRetryInterval(backup, len(tt.retryRecords))
            
            // Assert
            assert.Equal(t, tt.expectRetry, shouldRetry)
            if tt.expectRetry {
                assert.Equal(t, tt.expectInterval, interval)
            }
        })
    }
}
```

## 七、集成测试示例

```go
// tests/e2e/log_backup_retry_test.go

func TestLogBackupRetryE2E(t *testing.T) {
    // 1. 创建测试环境
    env := e2e.NewTestEnvironment(t)
    defer env.Cleanup()
    
    // 2. 部署TiDB集群
    tc := env.DeployTiDBCluster()
    
    // 3. 创建带重试策略的log backup
    backup := &v1alpha1.Backup{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-log-backup",
            Namespace: tc.Namespace,
        },
        Spec: v1alpha1.BackupSpec{
            Mode: v1alpha1.BackupModeLog,
            LogSubcommand: v1alpha1.LogStartCommand,
            BR: &v1alpha1.BRConfig{
                Cluster:   tc.Name,
                ClusterNamespace: tc.Namespace,
            },
            LogBackupRetryPolicy: &v1alpha1.LogBackupRetryPolicy{
                Enabled:         true,
                MaxRetryTimes:   3,
                InitialInterval: "10s",
            },
        },
    }
    
    // 4. 模拟失败场景
    env.InjectFailure("backup-job-fail", 2) // 前2次失败
    
    // 5. 创建backup
    err := env.Client.Create(context.TODO(), backup)
    require.NoError(t, err)
    
    // 6. 等待重试成功
    require.Eventually(t, func() bool {
        err := env.Client.Get(context.TODO(), 
            types.NamespacedName{Name: backup.Name, Namespace: backup.Namespace}, 
            backup)
        if err != nil {
            return false
        }
        
        // 检查是否重试成功
        return backup.Status.Phase == v1alpha1.BackupRunning &&
               len(backup.Status.LogBackupRetryRecords) == 2 &&
               backup.Status.LogBackupRetryRecords[1].Success == true
    }, 5*time.Minute, 10*time.Second)
    
    // 7. 验证内核状态
    kernelState := env.GetLogBackupKernelState(backup.Name)
    assert.Equal(t, "running", kernelState)
}
```

这些代码示例展示了Log Backup重试功能的核心实现，包括幂等性检查、重试逻辑、状态机管理和测试用例。实际实现时需要根据具体情况调整细节。
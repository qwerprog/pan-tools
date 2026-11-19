package uploader

import (
	"context"
	"errors"
	"github.com/oleiade/lane"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil"
	"os"
	"sync"
)

type (
	worker struct {
		id         int
		partOffset int64
		splitUnit  SplitUnit
		mu         sync.RWMutex
		checksum   string
	}

	workerList []*worker
)

func (wer *worker) getChecksum() string {
	wer.mu.RLock()
	defer wer.mu.RUnlock()
	return wer.checksum
}

func (wer *worker) setChecksum(cs string) {
	wer.mu.Lock()
	wer.checksum = cs
	wer.mu.Unlock()
}

// CheckSumList 返回所以worker的checksum
// TODO: 实现sort
func (werl *workerList) CheckSumList() []string {
	checksumList := make([]string, 0, len(*werl))
	for _, wer := range *werl {
		checksumList = append(checksumList, wer.getChecksum())
	}
	return checksumList
}

func (werl *workerList) Readed() int64 {
	var readed int64
	for _, wer := range *werl {
		readed += wer.splitUnit.Readed()
	}
	return readed
}

func (muer *MultiUploader) upload() (uperr error) {
	originPCSHost, err := muer.multiUpload.Precreate()
	if err != nil {
		return err
	}
	var (
		uploadDeque = lane.NewDeque()
	)

	var (
		checksumMap = make(map[int]string) // key: wer.id, value: checksum
		mu          sync.Mutex
	)

	// 加入队列
	for _, wer := range muer.workers {
		cs := wer.getChecksum()
		if cs == "" {
			uploadDeque.Append(wer)
		} else {
			checksumMap[wer.id] = cs
		}
	}

	for {
		wg := pcsutil.NewWaitGroup(muer.config.Parallel)
		for {
			if muer.isStopped() { // 优雅停止: 不再派发新分片
				break
			}

			e := uploadDeque.Shift()
			if e == nil { // 任务为空
				break
			}

			wer := e.(*worker)
			wg.AddDelta()
			if muer.isStopped() {
				uploadDeque.Prepend(wer)
				wg.Done()
				break
			}
			go func() {
				defer wg.Done()

				var (
					ctx, cancel = context.WithCancel(context.Background())
					doneChan    = make(chan struct{})
					checksum    string
					terr        error
					uploadID    = muer.getUploadID()
				)
				go func() {
					checksum, terr = muer.multiUpload.TmpFile(ctx, uploadID, muer.targetPath, wer.id, wer.partOffset, wer.splitUnit)
					close(doneChan)
				}()
				select {
				case <-muer.canceled:
					cancel()
					return
				case <-doneChan:
					// continue
				}
				cancel()
				if terr != nil {
					var me *MultiError
					if errors.As(terr, &me) {
						if me.Terminated { // 终止
							mu.Lock()
							if uperr == nil {
								uperr = me.Err
							}
							mu.Unlock()
							muer.closeCanceledOnce.Do(func() { // 只关闭一次
								close(muer.canceled)
							})
							return
						}
					}

					uploaderVerbose.Warnf("upload err: %s, id: %d\n", terr, wer.id)
					if muer.isStopped() { // 优雅停止: 失败的分片不再重试
						return
					}
					wer.splitUnit.Seek(0, os.SEEK_SET)
					uploadDeque.Append(wer)
					return
				}
				wer.setChecksum(checksum)
				mu.Lock()
				checksumMap[wer.id] = checksum // 记录成功任务的 checksum
				mu.Unlock()

				// 非阻塞通知更新，防止无状态监听或高并发完成时阻塞死锁
				if muer.updateInstanceStateChan != nil {
					select {
					case muer.updateInstanceStateChan <- struct{}{}:
					default:
					}
				}
			}()
		}
		wg.Wait()

		if muer.isStopped() { // 优雅停止: 跳出外层循环
			break
		}

		// 没有任务了
		if uploadDeque.Size() == 0 {
			break
		}
	}

	select {
	case <-muer.canceled:
		if uperr != nil {
			return uperr
		}
		return context.Canceled
	default:
	}

	if muer.isStopped() { // 优雅停止: 文件未完整上传, 不合并 superfile, 交由断点续传处理
		return context.Canceled
	}

	finalUploadID := muer.getUploadID()
	cerr := muer.multiUpload.CreateSuperFile(originPCSHost, muer.config.Policy, finalUploadID, muer.file.Len(), checksumMap)
	if cerr != nil {
		return cerr
	}

	return
}

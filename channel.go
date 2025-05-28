package main

import (
	"context"
	"sync"
	"time"
)

type MyMap struct {
	mp   map[int]int
	mpch map[int]chan struct{}
	sync.Mutex
}

func NewMyMap() *MyMap {
	return &MyMap{
		mp:   make(map[int]int),
		mpch: make(map[int]chan struct{}),
	}
}
func (m *MyMap) Put(k, v int) {
	m.Lock()
	defer m.Unlock()
	m.mp[k] = v
	//由于get方法中有读channel的操作，为了提醒get方法可以继续执行或通知get方法这个key已经存在，所以需要关闭channel
	if _, ok := m.mpch[k]; !ok {
		return
	}
	//不能直接关闭channel,多次关闭channel会导致panic，所以需要判断channel是否已经关闭（当channel关闭时会读到零值）
	select {
	case <-m.mpch[k]:
		return
	default:
		close(m.mpch[k])
	}
}
func (m *MyMap) Get(k int, maxWaitTime time.Duration) (int, error) {
	m.Lock()
	if v, ok := m.mp[k]; ok {
		m.Unlock()
		return v, nil
	}
	//此处存在读操作，如果单纯使用读写锁，会导致多个读协程同时进行写操作，所以需要互斥锁
	//此处使用互斥锁，是为了保证只有一个协程进行写操作
	//用map的key作为锁，保证同一个key只有一个channel
	//同一个key的多个读协程，只能共用一个channel（如果没有这个判断的话，第一个读操作创建后在下面释放锁，第二个读操作进来会把channel覆盖掉）
	if _, ok := m.mpch[k]; !ok {
		ch := make(chan struct{})
		m.mpch[k] = ch
	}
	ctx, cancel := context.WithTimeout(context.Background(), maxWaitTime)
	defer cancel()
	//提前释放锁，避免死锁 （这里如果不释放锁，channel会把这个过程锁死，就会造成互斥锁要等channel释放，而这个互斥锁是结构体共用的，写操作要拿锁才会写入channel）
	m.Unlock()
	//多路复用，监听多个channel，超时直接返错误值
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-m.mpch[k]:
	}
	m.Lock()
	v := m.mp[k]
	m.Unlock()
	return v, nil
}

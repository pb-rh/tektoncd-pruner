package helper

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	clockUtil "k8s.io/utils/clock"
	"knative.dev/pkg/logging"
)

type TTLResourceFuncs interface {
	Type() string
	Get(ctx context.Context, namespace, name string) (metav1.Object, error)
	Delete(ctx context.Context, namespace, name string) error
	Update(ctx context.Context, resource metav1.Object) error
	IsCompleted(resource metav1.Object) bool
	GetCompletionTime(resource metav1.Object) (metav1.Time, error)
	Ignore(resource metav1.Object) bool
	GetTTL(resource metav1.Object) *int32
}

type TTLQueue struct {
	// Jobs that the controller will check its TTL and attempt to delete when the TTL expires.
	queue        workqueue.RateLimitingInterface
	workersCount int
	clock        clockUtil.Clock // the clock for tracking time
	resourceFn   TTLResourceFuncs
}

func NewTTLQueue(name string, clock clockUtil.Clock, workersCount int, resourceFn TTLResourceFuncs) (*TTLQueue, error) {
	tq := &TTLQueue{
		clock:        clock,
		workersCount: workersCount,
		resourceFn:   resourceFn,
		queue: workqueue.NewRateLimitingQueueWithConfig(
			workqueue.DefaultControllerRateLimiter(),
			workqueue.RateLimitingQueueConfig{Name: name},
		),
	}
	if tq.resourceFn == nil {
		return nil, fmt.Errorf("resourceFunc interface can not be nil")
	}

	if tq.clock == nil {
		tq.clock = clockUtil.RealClock{}
	}

	if tq.workersCount == 0 {
		tq.workersCount = 1
	}

	return tq, nil
}

func (tq *TTLQueue) ProcessEvent(ctx context.Context, resource metav1.Object) error {
	logger := logging.FromContext(ctx)
	logger.Debugw("processing an event", "resource", tq.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName())

	// if the resource asked to ignored, remove from the queue as well
	if tq.resourceFn.Ignore(resource) {
		tq.RemoveFromQueue(ctx, resource.GetNamespace(), resource.GetName())
		return nil
	}

	// update ttl annotation
	err := tq.updateAnnotationTTLSeconds(ctx, resource)
	if err != nil {
		return err
	}

	if resource.GetDeletionTimestamp() == nil && tq.needsCleanup(resource) {
		tq.enqueue(logger, resource)
	}
	return nil
}
func (tq *TTLQueue) RemoveFromQueue(ctx context.Context, namespace, name string) {
	logger := logging.FromContext(ctx)
	logger.Debugw("removing a resource from the queue", "resource", tq.resourceFn.Type(), "namespace", namespace, "name", name)
	key := cache.ObjectName{Namespace: namespace, Name: name}
	tq.queue.Forget(key.String())
}

// Run starts the workers to clean up Jobs.
func (tq *TTLQueue) StartWorkers(ctx context.Context) {
	defer utilruntime.HandleCrash()
	defer tq.queue.ShutDown()

	logger := logging.FromContext(ctx)

	logger.Infow("starting TTL after finished controller",
		"workers", tq.workersCount, "forResource", tq.resourceFn.Type())
	defer logger.Infow("shutting down TTL after finished controller", "forResource", tq.resourceFn.Type())

	for i := 0; i < tq.workersCount; i++ {
		go wait.UntilWithContext(ctx, tq.worker, time.Second)
	}

	<-ctx.Done()
}

func (tq *TTLQueue) worker(ctx context.Context) {
	for tq.processNextWorkItem(ctx) {
	}
}

func (tq *TTLQueue) processNextWorkItem(ctx context.Context) bool {
	key, quit := tq.queue.Get()
	if quit {
		return false
	}
	defer tq.queue.Done(key)

	err := tq.processJob(ctx, key.(string))
	tq.handleErr(err, key)

	return true
}

func (tq *TTLQueue) handleErr(err error, key interface{}) {
	if err == nil {
		tq.queue.Forget(key)
		return
	}

	utilruntime.HandleError(fmt.Errorf("error cleaning up Resource %v, will retry: %v", key, err))
	tq.queue.AddRateLimited(key)
}

// processJob will check the Job's state and TTL and delete the Job when it
// finishes and its TTL after finished has expired. If the Job hasn't finished or
// its TTL hasn't expired, it will be added to the queue after the TTL is expected
// to expire.
// This function is not meant to be invoked concurrently with the same key.
func (tq *TTLQueue) processJob(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	logger := logging.FromContext(ctx)
	logger.Infow("checking if resource is ready for cleanup",
		"resourceType", tq.resourceFn.Type(),
		"resourceKey", key,
	)

	resource, err := tq.resourceFn.Get(ctx, namespace, name)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if expiredAt, err := tq.processTTL(logger, resource); err != nil {
		return err
	} else if expiredAt == nil {
		return nil
	}

	// The Job's TTL is assumed to have expired, but the Job TTL might be stale.
	// Before deleting the Job, do a final sanity check.
	// If TTL is modified before we do this check, we cannot be sure if the TTL truly expires.
	// The latest Job may have a different UID, but it's fine because the checks will be run again.
	freshObj, err := tq.resourceFn.Get(ctx, namespace, name)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	// Use the latest Job TTL to see if the TTL truly expires.
	expiredAt, err := tq.processTTL(logger, freshObj)
	if err != nil {
		return err
	} else if expiredAt == nil {
		return nil
	}

	// TODO: Cascade deletes the Jobs if TTL truly expires.
	// policy := metav1.DeletePropagationForeground
	// options := &client.DeleteOptions{
	// 	PropagationPolicy: &policy,
	// 	Preconditions:     &metav1.Preconditions{UID: &fresh.UID},
	// }
	logger.Info("cleaning up resource")
	if err := tq.resourceFn.Delete(ctx, namespace, name); err != nil {
		return err
	}
	// metrics.JobDeletionDurationSeconds.Observe(time.Since(*expiredAt).Seconds())
	return nil
}

func (tq *TTLQueue) updateAnnotationTTLSeconds(ctx context.Context, resource metav1.Object) error {
	logger := logging.FromContext(ctx)
	needsUpdate := false
	// get the annotation
	annotations := resource.GetAnnotations()
	if annotations == nil {
		needsUpdate = true
		annotations = map[string]string{}
	}
	if annotations[AnnotationTTLSecondsAfterFinished] == "" {
		needsUpdate = true
	}
	if needsUpdate {
		ttl := tq.resourceFn.GetTTL(resource)
		if ttl == nil {
			defaultTTL := DefaultTTLSeconds
			ttl = &defaultTTL
			logger.Info("unable to get the ttl, going with default ttl", "resource", resource)
		}
		annotations[AnnotationTTLSecondsAfterFinished] = strconv.Itoa(int(*ttl))
		resource.SetAnnotations(annotations)
		logger.Infow("updating ttl",
			"resource", tq.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName())
		return tq.resourceFn.Update(ctx, resource)
	}
	return nil
}

// processTTL checks whether a given Job's TTL has expired, and add it to the queue after the TTL is expected to expire
// if the TTL will expire later.
func (tq *TTLQueue) processTTL(logger *zap.SugaredLogger, resource metav1.Object) (expiredAt *time.Time, err error) {
	// We don't care about the Jobs that are going to be deleted, or the ones that don't need clean up.
	if resource.GetDeletionTimestamp() != nil || !tq.needsCleanup(resource) {
		return nil, nil
	}

	now := tq.clock.Now()
	t, e, err := tq.timeLeft(logger, resource, &now)
	if err != nil {
		return nil, err
	}

	// TTL has expired
	if *t <= 0 {
		return e, nil
	}

	tq.enqueueAfter(logger, resource, *t)
	return nil, nil
}

// needsCleanup checks whether a Job has finished and has a TTL set.
func (tq *TTLQueue) needsCleanup(resource metav1.Object) bool {
	if !tq.resourceFn.IsCompleted(resource) {
		return false
	}
	// get the annotation
	annotations := resource.GetAnnotations()
	if annotations == nil {
		return false
	}
	if annotations[AnnotationTTLSecondsAfterFinished] == "" {
		return false
	}
	return true
}

func (tq *TTLQueue) timeLeft(logger *zap.SugaredLogger, j metav1.Object, since *time.Time) (*time.Duration, *time.Time, error) {
	finishAt, expireAt, err := tq.getFinishAndExpireTime(j)
	if err != nil {
		return nil, nil, err
	}

	if finishAt.After(*since) {
		logger.Warn("Warning: Found Job finished in the future. This is likely due to time skew in the cluster. Resource cleanup will be deferred.")
	}
	remaining := expireAt.Sub(*since)
	logger.Infow("found Job finished", "finishTime", finishAt.UTC(), "remainingTTL", remaining, "startTime", since.UTC(), "deadlineTTL", expireAt.UTC())
	return &remaining, expireAt, nil
}

func (tq *TTLQueue) getFinishAndExpireTime(resource metav1.Object) (*time.Time, *time.Time, error) {
	if !tq.needsCleanup(resource) {
		return nil, nil, fmt.Errorf("job %s/%s should not be cleaned up", resource.GetNamespace(), resource.GetName())
	}
	t, err := tq.resourceFn.GetCompletionTime(resource)
	if err != nil {
		return nil, nil, err
	}
	finishAt := t.Time
	// get ttl duration
	ttlDuration, err := tq.getTTLSeconds(resource)
	if err != nil {
		return nil, nil, err
	}
	expireAt := finishAt.Add(*ttlDuration)
	return &finishAt, &expireAt, nil
}

func (tq *TTLQueue) getTTLSeconds(resource metav1.Object) (*time.Duration, error) {
	ttlString := resource.GetAnnotations()[AnnotationTTLSecondsAfterFinished]
	ttl, err := strconv.Atoi(ttlString)
	if err != nil {
		return nil, err
	}
	ttlDuration := time.Duration(ttl) * time.Second
	return &ttlDuration, nil
}

func (tq *TTLQueue) enqueue(logger *zap.SugaredLogger, resource metav1.Object) {
	logger.Infow("adding a resource to cleanup",
		"resource", tq.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName())
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(resource)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", resource, err))
		return
	}
	tq.queue.Add(key)
	logger.Infow("added a resource to cleanup",
		"resource", tq.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName())
}

func (tq *TTLQueue) enqueueAfter(logger *zap.SugaredLogger, resource metav1.Object, after time.Duration) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(resource)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", resource, err))
		return
	}
	tq.queue.AddAfter(key, after)
	logger.Infow("added a resource to cleanup with after",
		"resource", tq.resourceFn.Type(), "namespace", resource.GetNamespace(), "name", resource.GetName(), "waitDuration", after)
}

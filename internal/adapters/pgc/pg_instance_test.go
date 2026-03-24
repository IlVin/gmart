package pgc

import (
	"context"
	"net"
	"testing"
	"time"

	"gmart/internal/adapters/pgc/backoff"
	"gmart/internal/adapters/pgc/fcounter"
	"gmart/internal/domain"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestPgInstance_OnlineOffline(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetrics := NewMockPgInstanceMetrics(ctrl)
	mockPool := NewMockpgxPoolDriverIface(ctrl)

	h := &pgInstance{
		instanceName: "test_db",
		pgPool:       mockPool,
		metrics:      mockMetrics,
	}

	t.Run("Transition to Online", func(t *testing.T) {
		mockMetrics.EXPECT().SetStatus("test_db", true).Times(1)
		h.Online()
		assert.True(t, h.IsReady())
	})

	t.Run("Transition to Offline", func(t *testing.T) {
		mockMetrics.EXPECT().SetStatus("test_db", false).Times(1)
		mockMetrics.EXPECT().IncOfflineEvent("test_db").Times(1)
		h.Offline()
		assert.False(t, h.IsReady())
	})
}

func TestPgInstance_Tx_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetrics := NewMockPgInstanceMetrics(ctrl)
	mockPool := NewMockpgxPoolDriverIface(ctrl)
	mockTx := NewMockTx(ctrl)

	h := &pgInstance{
		instanceName: "test_db",
		pgPool:       mockPool,
		metrics:      mockMetrics,
		failures:     fcounter.NewFailureCounter(3, time.Second),
		repeater:     backoff.NewPgBackoff(1, time.Millisecond),
	}
	h.isReady.Store(true)

	ctx := context.Background()

	mockPool.EXPECT().Begin(ctx).Return(mockTx, nil)
	mockTx.EXPECT().Commit(gomock.Any()).Return(nil)
	mockMetrics.EXPECT().ObserveLatency("test_db", domain.OpTx, gomock.Any())
	mockMetrics.EXPECT().SetStatus("test_db", true).AnyTimes() // Из-за HandleError(nil)

	err := h.Tx(ctx, func(ctx context.Context, tx PgxTxIface) error {
		return nil
	})

	assert.NoError(t, err)
}

func TestPgInstance_HandleError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetrics := NewMockPgInstanceMetrics(ctrl)
	h := &pgInstance{
		instanceName: "test_db",
		metrics:      mockMetrics,
		failures:     fcounter.NewFailureCounter(3, time.Second),
	}
	h.isReady.Store(true)

	t.Run("Nil error keeps it online", func(t *testing.T) {
		mockMetrics.EXPECT().SetStatus("test_db", true).AnyTimes()
		err := h.HandleError(nil) // Теперь Reset() не упадет
		assert.NoError(t, err)
	})
}

func TestPgInstance_CanTry_Online(t *testing.T) {
	h := &pgInstance{}
	h.isReady.Store(false)
	h.lastRetry.Store(0)

	t.Run("Allow first try in offline", func(t *testing.T) {
		assert.True(t, h.CanTry())
	})

	t.Run("Block subsequent tries immediately", func(t *testing.T) {
		h.lastRetry.Store(time.Now().Unix())
		assert.False(t, h.CanTry())
	})
}

// Тест механизма CanTry в режиме Offline
func TestPgInstance_CanTry_Offline(t *testing.T) {
	h := &pgInstance{}
	h.isReady.Store(false)                    // Принудительно Offline
	h.lastRetry.Store(time.Now().Unix() - 10) // Последняя попытка была давно

	t.Run("allows one try after interval", func(t *testing.T) {
		allowed := h.CanTry()
		assert.True(t, allowed)

		// Сразу второй раз нельзя
		allowedAgain := h.CanTry()
		assert.False(t, allowedAgain)
	})
}

// Тест логики переключения состояний (Online -> Offline)
func TestPgInstance_CircuitBreaker(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPool := NewMockpgxPoolDriverIface(ctrl)

	// Создаем инстанс вручную, чтобы не подключаться к реальной БД
	h := &pgInstance{
		pgPool:       mockPool,
		instanceName: "test-db",
		failures:     fcounter.NewFailureCounter(2, 1*time.Second), // 2 ошибки и мы в ауте
	}
	h.isReady.Store(true)

	t.Run("transitions to offline after failures", func(t *testing.T) {
		networkErr := net.ErrClosed

		// Первая ошибка — еще Online
		h.HandleError(networkErr)
		assert.True(t, h.IsReady())

		// Вторая ошибка — должен уйти в Offline
		h.HandleError(networkErr)
		assert.False(t, h.IsReady())
	})

	t.Run("transitions back to online after success", func(t *testing.T) {
		h.HandleError(nil)
		assert.True(t, h.IsReady())
	})
}

// Тест защиты от паники в коллбеках
func TestPgInstance_PanicRecovery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPool := NewMockpgxPoolDriverIface(ctrl)

	// Инициализируем зависимости, чтобы WithRetry не падал
	h := &pgInstance{
		pgPool:   mockPool,
		repeater: backoff.NewPgBackoff(1, 1*time.Millisecond), // 1 попытка, без задержки
		failures: fcounter.NewFailureCounter(10, time.Second),
	}
	h.isReady.Store(true)

	t.Run("recovery in PgPool", func(t *testing.T) {
		err := h.PgPool(context.Background(), func(ctx context.Context, pool PgxPoolIface) error {
			panic("something went wrong inside callback")
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "panic recovered")
	})
}

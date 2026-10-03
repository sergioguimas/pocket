package app

import (
	"context"
	"fmt"

	"pocket/internal/domain"
	"pocket/internal/store"
)

// Pular marca uma ocorrência "aberta" como "pulada" (§3.3 regra 4): não
// conta na taxa nem quebra a sequência; desfazível por Despular.
func (a *App) Pular(ctx context.Context, ocorrenciaID int64) (ItemComMetricas, error) {
	var itemFinal domain.Item
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		oc, err := tx.OcorrenciaBuscar(ctx, ocorrenciaID)
		if err != nil {
			return mapearErroStore(err, fmt.Sprintf("ocorrência %d não encontrada", ocorrenciaID))
		}
		if oc.Status != domain.StatusAberta {
			return erroConflito("só dá para pular uma ocorrência aberta")
		}
		item, err := tx.ItemBuscar(ctx, oc.ItemID)
		if err != nil {
			return err
		}

		oc.Status = domain.StatusPulada
		if err := tx.OcorrenciaAtualizar(ctx, oc); err != nil {
			return err
		}
		a.obs.OcorrenciaMudou(tx, oc)
		itemFinal = item
		return nil
	})
	if err != nil {
		return ItemComMetricas{}, err
	}
	return a.itemComMetricas(ctx, itemFinal)
}

// Despular reverte Pular, recalculando o status como se a ocorrência nunca
// tivesse sido pulada (domain.Despular olha os registros já existentes no
// intervalo dela).
func (a *App) Despular(ctx context.Context, ocorrenciaID int64) (ItemComMetricas, error) {
	var itemFinal domain.Item
	err := a.st.WithTx(ctx, func(tx *store.Tx) error {
		oc, err := tx.OcorrenciaBuscar(ctx, ocorrenciaID)
		if err != nil {
			return mapearErroStore(err, fmt.Sprintf("ocorrência %d não encontrada", ocorrenciaID))
		}
		if oc.Status != domain.StatusPulada {
			return erroConflito("só dá para despular uma ocorrência pulada")
		}
		item, err := tx.ItemBuscar(ctx, oc.ItemID)
		if err != nil {
			return err
		}
		registros, err := tx.RegistrosPorItem(ctx, item.ID)
		if err != nil {
			return err
		}
		n := contarRegistrosNoIntervalo(registros, oc, a.loc)
		oc.Status = domain.Despular(item, oc, n, a.hoje())
		if err := tx.OcorrenciaAtualizar(ctx, oc); err != nil {
			return err
		}
		a.obs.OcorrenciaMudou(tx, oc)
		itemFinal = item
		return nil
	})
	if err != nil {
		return ItemComMetricas{}, err
	}
	return a.itemComMetricas(ctx, itemFinal)
}

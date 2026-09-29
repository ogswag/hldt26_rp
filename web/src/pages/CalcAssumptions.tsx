import { useMemo, useState } from "react";

import type { AssumptionSet, AssumptionView, CalculateResult, EconOverrides, NormValue, SharedCost } from "../api/client";
import { formatNum, formatRub, overrideFieldLabel, pickedPrice } from "../econ/view";
import { numberText } from "../ui/numberText";
import {
  activeAssumptionEdit,
  addAssumptionSetEdit,
  defaultDiscountRate,
  addSharedCostEdit,
  assumptionSetEdit,
  assumptionSetsSelector,
  deleteAssumptionSetEdit,
  deleteSharedCostEdit,
  sharedCostEdit,
  sharedCostsSelector,
  type Edit,
} from "../projects/econRecords";
import { setFieldLabel, whatIfLabels } from "../projects/assumptionFields";
import { uuid } from "../store/clientId";
import { reasonText } from "../store/reasons";
import { useProjectStore, useStoreSelector, useUndoHistory } from "../store/useProjectStore";
import { FieldRow } from "../ui/FieldRow";
import { noData } from "../ui/noData";
import { NumberField } from "../ui/NumberField";
import { Reflow } from "../ui/Reflow";
import { reveal } from "../ui/reveal";
import { Select, type SelectOption } from "../ui/Select";
import { TextField } from "../ui/TextField";
import { UnitField } from "../ui/UnitField";

const bucketOptions: SelectOption<SharedCost["bucket"]>[] = [
  { value: "capex", label: "CAPEX" },
  { value: "opex", label: "OPEX/год" },
];

// percent shows a share (0,2) the way the rest of the app does, as percent (20).
function percent(share: number): number {
  return Math.round(share * 10000) / 100;
}

// overrideText shows a what-if value the way its field does: rubles, or a factor as percent.
function overrideText(field: string, v: number): string {
  if (field === "price_rub") {
    return formatRub(v);
  }
  return field.endsWith("_factor") ? `${formatNum(percent(v), 2)}%` : formatNum(v, 4);
}

// Assumptions is the Допущения subtab: the what-if, then, in a saved project, the assumption sets and the shared
// infrastructure costs. Settings are field rows as on Объект; costs, a list of like items, are a table.
export function Assumptions({
  result,
  stored,
  projectId,
  onRun,
  onRecalc,
}: {
  result: CalculateResult;
  // stored is the what-if the project holds now; the result on screen may still be the one before it.
  stored: EconOverrides;
  projectId: string | undefined;
  onRun: (ov: EconOverrides) => Promise<void>;
  onRecalc: () => void;
}) {
  return (
    <>
      {/* A guest has no set editor, so the set in use is named here. */}
      {!projectId && result.assumption_set ? (
        <p>
          <Reflow>
            Набор допущений: {result.assumption_set.name}. НДС{" "}
            {formatNum(result.assumption_set.vat_rate * 100, 0)}%,
            {result.assumption_set.prices_include_vat ? " цены с НДС" : " цены без НДС"}
            {result.assumption_set.vat_recoverable ? ", НДС к вычету" : ""}. Доля денежной экономии
            труда {formatNum(result.assumption_set.labor_cash_share * 100, 0)}%.
          </Reflow>
        </p>
      ) : null}
      <WhatIf
        key={`${result.solution_name ?? ""}:${JSON.stringify(result.overrides ?? [])}:${result.scenarios[1]?.capex_rub ?? ""}`}
        result={result}
        stored={stored}
        onRun={onRun}
      />
      {result.overrides && result.overrides.length > 0 ? (
        <section className="param-section" aria-labelledby="overrides-h">
          <h2 id="overrides-h">Переопределения</h2>
          <ul>
            {result.overrides.map((o) => (
              <li key={o.field}>
                <Reflow>
                  {overrideFieldLabel(o.field)}: задано {overrideText(o.field, o.value)}, модель
                  считала {overrideText(o.field, o.computed)}
                </Reflow>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {projectId ? (
        <ProjectEconEditors projectId={projectId} view={result.assumption_set} onSaved={onRecalc} />
      ) : null}
      {result.assumptions.length > 0 ? (
        <section>
          <h2>Допущения</h2>
          <ul>
            {result.assumptions.map((assumption) => (
              <li key={assumption}>
                <Reflow>{assumption}</Reflow>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </>
  );
}

function WhatIf({
  result,
  stored,
  onRun,
}: {
  result: CalculateResult;
  stored: EconOverrides;
  onRun: (ov: EconOverrides) => Promise<void>;
}) {
  const catalog = pickedPrice(result);

  // Each save recalculates on the server, so these fields save on Enter and on leaving the field, not on a pause while typing.
  function apply(patch: EconOverrides) {
    const ov: EconOverrides = { ...stored, ...patch };
    for (const k of Object.keys(ov) as (keyof EconOverrides)[]) {
      if (ov[k] === undefined) {
        delete ov[k];
      }
    }
    if (ov.price_rub !== undefined && catalog !== null && ov.price_rub === catalog) {
      delete ov.price_rub;
    }
    if (ov.volume_factor === 1) {
      delete ov.volume_factor;
    }
    if (ov.labor_factor === 1) {
      delete ov.labor_factor;
    }
    // A failed run is reported in the corner notice; the numbers on the page stay.
    void onRun(ov).catch(() => {});
  }

  const priceHelp = `${catalog !== null ? `В каталоге: ${formatRub(catalog)}. ` : ""}Пустое поле возвращает цену каталога.`;
  return (
    <section className="param-section" aria-labelledby="whatif-h">
      <h2 id="whatif-h">Что если</h2>
      <div className="field-rows is-quiet">
        <FieldRow
          id="whatif-price"
          data-search-id="whatif:price"
          label={whatIfLabels.price}
          unit="₽"
          help={priceHelp}
        >
          <UnitField unit="₽">
            <NumberField
              id="whatif-price"
              saveOnPause={false}
              optional
              placeholder={noData}
              value={stored.price_rub ?? catalog}
              valid={(n) => n >= 0}
              onCommit={(n) => apply({ price_rub: n })}
            />
          </UnitField>
        </FieldRow>
        <FieldRow
          id="whatif-volume"
          data-search-id="whatif:volume"
          label={whatIfLabels.volume}
          unit="% от параметров"
          help="100% означает объём из параметров объекта."
        >
          <UnitField unit="%">
            <NumberField
              id="whatif-volume"
              saveOnPause={false}
              value={percent(stored.volume_factor ?? 1)}
              valid={(n) => n > 0}
              onCommit={(n) => apply({ volume_factor: n / 100 })}
            />
          </UnitField>
        </FieldRow>
        <FieldRow
          id="whatif-labor"
          data-search-id="whatif:labor"
          label={whatIfLabels.labor}
          unit="% от параметров"
          help="100% означает ФОТ из параметров объекта."
        >
          <UnitField unit="%">
            <NumberField
              id="whatif-labor"
              saveOnPause={false}
              value={percent(stored.labor_factor ?? 1)}
              valid={(n) => n > 0}
              onCommit={(n) => apply({ labor_factor: n / 100 })}
            />
          </UnitField>
        </FieldRow>
      </div>
    </section>
  );
}

type NormKey = keyof Pick<
  AssumptionSet,
  | "utilization"
  | "availability"
  | "reserve"
  | "service_share"
  | "delivery_share"
  | "comm_rub_per_robot_year"
  | "technician_wage_month_rub"
>;

const normRows: {
  key: NormKey;
  field: "utilization" | "availability" | "reserve" | "service" | "delivery" | "comm" | "wage";
  unit: string;
  share: boolean;
  min: number;
  max: number;
  help: string;
}[] = [
  {
    key: "utilization",
    field: "utilization",
    unit: "%",
    share: true,
    min: 1,
    max: 100,
    help: "Доля времени, когда робот занят работой. Пустое поле берёт норматив.",
  },
  {
    key: "availability",
    field: "availability",
    unit: "%",
    share: true,
    min: 1,
    max: 100,
    help: "Доля времени, когда робот доступен. Пустое поле берёт норматив.",
  },
  {
    key: "reserve",
    field: "reserve",
    unit: "%",
    share: true,
    min: 0,
    max: 100,
    help: "Запас роботов сверх расчётного числа. Пустое поле берёт норматив.",
  },
  {
    key: "service_share",
    field: "service",
    unit: "%",
    share: true,
    min: 0,
    max: 100,
    help: "Доля цены оборудования на сервис и ремонт в год. Пустое поле берёт ТТХ робота или норматив.",
  },
  {
    key: "delivery_share",
    field: "delivery",
    unit: "%",
    share: true,
    min: 0,
    max: 100,
    help: "Доля цены оборудования на доставку. Пустое поле берёт норматив.",
  },
  {
    key: "comm_rub_per_robot_year",
    field: "comm",
    unit: "₽",
    share: false,
    min: 0,
    max: 10_000_000,
    help: "Связь на одного робота в год, с НДС. Пустое поле берёт норматив.",
  },
  {
    key: "technician_wage_month_rub",
    field: "wage",
    unit: "₽",
    share: false,
    min: 0,
    max: 10_000_000,
    help: "Зарплата техника в месяц, без НДС. Пустое поле берёт норматив.",
  },
];

function normShown(share: boolean, value: number): number {
  return share ? percent(value) : value;
}

function ProjectEconEditors({
  projectId,
  view,
  onSaved,
}: {
  projectId: string;
  view: AssumptionView | undefined;
  onSaved: () => void;
}) {
  const store = useProjectStore(projectId);
  const [history] = useUndoHistory(projectId);
  const selectCosts = useMemo(() => sharedCostsSelector(), []);
  const selectSets = useMemo(() => assumptionSetsSelector(), []);
  const costs = useStoreSelector(store, selectCosts);
  const sets = useStoreSelector(store, selectSets);
  const ready = useStoreSelector(store, () => store.ready);
  const [error, setError] = useState("");

  // run sends one edit as one transaction and records it for ctrl+Z. A recalculation follows, because these
  // numbers go straight into the economics; so the fields here save on Enter and on leaving the field.
  function run(edit: Edit): void {
    if (edit.ops.length === 0) {
      return;
    }
    const r = history.run(edit.label, edit.ops);
    if (r.outcome.status === "rejected") {
      setError(`Изменение не применено. ${reasonText(r.outcome.reason)}`);
      return;
    }
    setError("");
    onSaved();
  }

  // addSet adds a set at the end and takes the user to its name, the field they came to fill in.
  function addSet(): void {
    const id = uuid();
    run(
      addAssumptionSetEdit(store.getState(), {
        id,
        name: `Набор ${sets.length + 1}`,
        is_active: false,
        vat_rate: 0.22,
        prices_include_vat: true,
        vat_recoverable: false,
        labor_cash_share: 1,
        discount_rate: defaultDiscountRate,
        sort_order: sets.length,
      }),
    );
    void reveal(`set:${id}:name`, { focus: true });
  }

  if (!ready) {
    return null;
  }

  const active = sets.find((a) => a.is_active);
  const setOptions: SelectOption[] = sets
    .filter((a) => a.id)
    .map((a) => ({ value: a.id as string, label: a.name }));

  return (
    <>
      {error ? (
        <p className="error">
          <Reflow>{error}</Reflow>
        </p>
      ) : null}
      {sets.length > 0 ? (
        <section className="param-section" aria-labelledby="sets-h" data-search-id="section:sets">
          <h2 id="sets-h">Наборы допущений</h2>
          <div className="field-rows is-quiet">
            <FieldRow
              id="active-set"
              data-search-id="set:active"
              label="Активный набор"
              help="Этот набор идёт в расчёт."
            >
              <Select
                id="active-set"
                value={active?.id ?? ""}
                options={setOptions}
                onChange={(id) => run(activeAssumptionEdit(store.getState(), id))}
              />
            </FieldRow>
          </div>
        </section>
      ) : null}
      {sets.map((a, i) => {
        const key = a.id ?? String(i);
        const row = (field: string) => ({
          id: `set-${key}-${field}`,
          "data-search-id": `set:${key}:${field}`,
        });
        return (
          <section key={key} className="param-section" aria-labelledby={`set-${key}-h`}>
            <h2 id={`set-${key}-h`}>Набор «{a.name}»</h2>
            <div className="field-rows is-quiet">
              <FieldRow {...row("name")} label={setFieldLabel("name")}>
                <TextField
                  saveOnPause={false}
                  id={`set-${key}-name`}
                  maxLength={120}
                  value={a.name}
                  onCommit={(name) =>
                    a.id && name && run(assumptionSetEdit(store.getState(), a.id, { name }))
                  }
                />
              </FieldRow>
              <FieldRow {...row("vat")} label={setFieldLabel("vat")} unit="%">
                <UnitField unit="%">
                  <NumberField
                    saveOnPause={false}
                    id={`set-${key}-vat`}
                    value={percent(a.vat_rate)}
                    valid={(n) => n >= 0 && n <= 100}
                    onCommit={(n) =>
                      a.id && run(assumptionSetEdit(store.getState(), a.id, { vat_rate: n / 100 }))
                    }
                  />
                </UnitField>
              </FieldRow>
              <FieldRow {...row("incl")} label={setFieldLabel("incl")}>
                <input
                  id={`set-${key}-incl`}
                  type="checkbox"
                  checked={a.prices_include_vat}
                  onChange={(e) =>
                    a.id &&
                    run(
                      assumptionSetEdit(store.getState(), a.id, {
                        prices_include_vat: e.target.checked,
                      }),
                    )
                  }
                />
              </FieldRow>
              <FieldRow
                {...row("recover")}
                label={setFieldLabel("recover")}
                help="Если НДС к вычету, в денежный расчёт идёт цена без НДС."
              >
                <input
                  id={`set-${key}-recover`}
                  type="checkbox"
                  checked={a.vat_recoverable}
                  onChange={(e) =>
                    a.id &&
                    run(
                      assumptionSetEdit(store.getState(), a.id, {
                        vat_recoverable: e.target.checked,
                      }),
                    )
                  }
                />
              </FieldRow>
              <FieldRow
                {...row("labor")}
                label={setFieldLabel("labor")}
                fullLabel="Доля денежной экономии труда"
                unit="%"
                help="Какая доля высвобождённого ФОТ считается живыми деньгами."
              >
                <UnitField unit="%">
                  <NumberField
                    saveOnPause={false}
                    id={`set-${key}-labor`}
                    value={percent(a.labor_cash_share)}
                    valid={(n) => n >= 0 && n <= 100}
                    onCommit={(n) =>
                      a.id &&
                      run(assumptionSetEdit(store.getState(), a.id, { labor_cash_share: n / 100 }))
                    }
                  />
                </UnitField>
              </FieldRow>
              <FieldRow
                {...row("discount")}
                label={setFieldLabel("discount")}
                fullLabel="Ставка дисконтирования для NPV и IRR"
                unit="%"
                help="Годовая ставка приведения денежных потоков к сегодняшним деньгам. Расчёт в постоянных ценах, без инфляции. Простую окупаемость она не меняет."
              >
                <UnitField unit="%">
                  <NumberField
                    saveOnPause={false}
                    id={`set-${key}-discount`}
                    value={percent(a.discount_rate)}
                    valid={(n) => n >= 1 && n <= 100}
                    onCommit={(n) =>
                      a.id &&
                      run(assumptionSetEdit(store.getState(), a.id, { discount_rate: n / 100 }))
                    }
                  />
                </UnitField>
              </FieldRow>
              {normRows.map((spec) => {
                const stored = a[spec.key];
                const effective = view?.[spec.key] as NormValue | undefined;
                const byNorm = stored == null;
                const help = byNorm ? `${spec.help} Сейчас по нормативу.` : `${spec.help} Очистите поле, чтобы вернуть норматив.`
                return (
                  <FieldRow
                    key={spec.key}
                    {...row(spec.field)}
                    label={setFieldLabel(spec.field)}
                    unit={spec.unit}
                    help={help}
                  >
                    <UnitField unit={spec.unit} className={byNorm ? "is-norm" : undefined}>
                      <NumberField
                        saveOnPause={false}
                        optional
                        id={`set-${key}-${spec.field}`}
                        placeholder={
                          effective ? numberText(normShown(spec.share, effective.value)) : undefined
                        }
                        value={byNorm ? null : normShown(spec.share, stored)}
                        valid={(n) => n >= spec.min && n <= spec.max}
                        onCommit={(n) =>
                          a.id &&
                          run(
                            assumptionSetEdit(store.getState(), a.id, {
                              [spec.key]: n === undefined ? null : spec.share ? n / 100 : n,
                            }),
                          )
                        }
                      />
                    </UnitField>
                  </FieldRow>
                );
              })}
            </div>
            {sets.length > 1 ? (
              <div className="section-actions">
                <button
                  type="button"
                  className="btn btn-danger"
                  onClick={() => a.id && run(deleteAssumptionSetEdit(store.getState(), a.id))}
                >
                  Удалить набор
                </button>
              </div>
            ) : null}
          </section>
        );
      })}
      <div className="section-actions">
        <button type="button" data-search-id="action:add-set" onClick={addSet}>
          Добавить набор
        </button>
      </div>

      <section className="param-section" aria-labelledby="costs-h">
        <h2 id="costs-h">Общая инфраструктура</h2>
        {costs.length > 0 ? (
          <div className="table-wrap">
            <table className="edit-rows is-quiet">
              <thead>
                <tr>
                  <th>Статья</th>
                  <th>Тип</th>
                  <th className="num">Сумма</th>
                  <th>
                    <span className="sr-only">Действия</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {costs.map((c, i) => (
                  <tr key={c.id ?? i} data-search-id={`cost:${c.id ?? i}`}>
                    <td>
                      <TextField
                        saveOnPause={false}
                        aria-label={`Статья ${i + 1}: название`}
                        maxLength={120}
                        value={c.label}
                        onCommit={(label) =>
                          c.id && label && run(sharedCostEdit(store.getState(), c.id, { label }))
                        }
                      />
                    </td>
                    <td>
                      <Select
                        aria-label={`${c.label}: тип статьи`}
                        value={c.bucket}
                        options={bucketOptions}
                        onChange={(bucket) =>
                          c.id && run(sharedCostEdit(store.getState(), c.id, { bucket }))
                        }
                      />
                    </td>
                    <td className="num">
                      <UnitField unit="₽">
                        <NumberField
                          saveOnPause={false}
                          aria-label={`${c.label}: сумма, ₽`}
                          value={c.rub}
                          valid={(n) => n >= 0}
                          onCommit={(n) =>
                            c.id && run(sharedCostEdit(store.getState(), c.id, { rub: n }))
                          }
                        />
                      </UnitField>
                    </td>
                    <td>
                      <button
                        type="button"
                        className="btn btn-danger"
                        onClick={() => c.id && run(deleteSharedCostEdit(c.id))}
                      >
                        Удалить
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
        <div className="section-actions">
          <button
            type="button"
            data-search-id="action:add-cost"
            onClick={() => {
              const id = uuid();
              run(
                addSharedCostEdit(store.getState(), {
                  id,
                  code: `shared_${id}`,
                  label: "Общая инфраструктура",
                  bucket: "capex",
                  rub: 0,
                  sort_order: costs.length,
                }),
              );
              void reveal(`cost:${id}`, { focus: true });
            }}
          >
            Добавить статью
          </button>
        </div>
      </section>
    </>
  );
}

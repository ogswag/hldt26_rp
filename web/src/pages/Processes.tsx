import { useQuery } from "@tanstack/react-query";
import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";

import { fetchProject, type Process } from "../api/client";
import { useProjectRoute } from "../layout/route";
import { unitText } from "../params/unit";
import { taskTypeOptions } from "../projects/defaults";
import { processFields, type ProcessFieldKey } from "../projects/processFields";
import {
  addProcessEdit,
  deleteProcessEdit,
  processFieldEdit,
  processesSelector,
  type Edit,
  type ProcessPatch,
} from "../projects/processRecords";
import { uuid } from "../store/clientId";
import { useOnSearchTarget } from "../search/target";
import { reasonText } from "../store/reasons";
import {
  usePresence,
  useProjectStore,
  useReportSelection,
  useStoreSelector,
  useUndoHistory,
} from "../store/useProjectStore";
import { ActionBar } from "../ui/ActionBar";
import { FieldRow } from "../ui/FieldRow";
import { CloseIcon } from "../ui/icons";
import { NumberField } from "../ui/NumberField";
import { noData } from "../ui/noData";
import { numberText } from "../ui/numberText";
import { Reflow } from "../ui/Reflow";
import { Select } from "../ui/Select";
import { TextField } from "../ui/TextField";
import { UnitField } from "../ui/UnitField";

// processLines are the lines under a process card's name: demand, time limits and staff.
function processLines(p: Process): string[] {
  const demand = p.demand.units_per_day ?? 0;
  // NOTE: the unit comes from data in the singular, so it stays out of number agreement.
  return [
    `${numberText(demand)} в сутки, единица: ${unitText(p.demand.unit || "ед")}`,
    `Ожидание до ${p.sla.max_wait_min ?? 0} мин, цикл до ${p.sla.max_cycle_min ?? 0} мин`,
    `Персонал базы ${p.baseline_staff.headcount ?? 0} чел`,
  ];
}

// freeCode gives a new process a code no other process in the list uses; the schema keeps codes unique.
function freeCode(items: readonly Process[]): string {
  const taken = new Set(items.map((p) => p.code));
  for (let n = items.length + 1; ; n++) {
    const code = `process_${n}`;
    if (!taken.has(code)) {
      return code;
    }
  }
}

// ProcessDialog edits one process. Fields save themselves, so the only button closes it.
function ProcessDialog({
  p,
  onClose,
  update,
}: {
  p: Process;
  onClose: () => void;
  update: (patch: ProcessPatch) => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const uid = useId();
  useEffect(() => {
    const d = ref.current;
    if (d && !d.open) {
      d.showModal();
    }
  }, []);

  const inputs: Record<ProcessFieldKey, (id: string) => ReactNode> = {
    name: (id) => (
      <TextField
        id={id}
        maxLength={200}
        value={p.name}
        onCommit={(name) => name && update({ name })}
      />
    ),
    task_type: (id) => (
      <Select
        id={id}
        value={p.task_type}
        options={taskTypeOptions}
        onChange={(task_type) => update({ task_type })}
      />
    ),
    units_per_day: (id) => (
      <NumberField
        id={id}
        value={p.demand.units_per_day}
        valid={(n) => n >= 0 && n <= 10_000_000}
        onCommit={(n) => update({ demand: { ...p.demand, units_per_day: n } })}
      />
    ),
    units_per_job: (id) => (
      <NumberField
        id={id}
        optional
        placeholder={noData}
        value={p.demand.units_per_job}
        valid={(n) => n > 0 && n <= 10_000}
        onCommit={(n) => update({ demand: { ...p.demand, units_per_job: n } })}
      />
    ),
    max_wait_min: (id) => (
      <NumberField
        id={id}
        optional
        placeholder={noData}
        value={p.sla.max_wait_min}
        valid={(n) => n >= 0}
        onCommit={(n) => update({ sla: { ...p.sla, max_wait_min: n } })}
      />
    ),
    max_cycle_min: (id) => (
      <NumberField
        id={id}
        optional
        placeholder={noData}
        value={p.sla.max_cycle_min}
        valid={(n) => n >= 0}
        onCommit={(n) => update({ sla: { ...p.sla, max_cycle_min: n } })}
      />
    ),
    priority: (id) => (
      <NumberField
        id={id}
        value={p.sla.priority ?? 0}
        valid={(n) => Number.isInteger(n) && n >= 0 && n <= 9}
        onCommit={(priority) => update({ sla: { ...p.sla, priority } })}
      />
    ),
    load_s: (id) => (
      <NumberField
        id={id}
        optional
        placeholder={noData}
        value={p.durations.load_s}
        valid={(n) => n >= 0}
        onCommit={(n) => update({ durations: { ...p.durations, load_s: n } })}
      />
    ),
    unload_s: (id) => (
      <NumberField
        id={id}
        optional
        placeholder={noData}
        value={p.durations.unload_s}
        valid={(n) => n >= 0}
        onCommit={(n) => update({ durations: { ...p.durations, unload_s: n } })}
      />
    ),
    headcount: (id) => (
      <NumberField
        id={id}
        value={p.baseline_staff.headcount}
        valid={(n) => n >= 0}
        onCommit={(headcount) => update({ baseline_staff: { ...p.baseline_staff, headcount } })}
      />
    ),
  };

  return (
    <dialog
      ref={ref}
      className="dialog dialog-wide"
      aria-labelledby={`${uid}-title`}
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
    >
      <div className="dialog-head">
        <h2 id={`${uid}-title`}>{p.name || "Процесс"}</h2>
        <button
          type="button"
          className="icon-button"
          aria-label="Закрыть"
          title="Закрыть"
          onClick={onClose}
        >
          <CloseIcon size={16} />
        </button>
      </div>
      <div className="field-rows is-quiet">
        {processFields.map((f) => {
          const id = `${uid}-${f.key}`;
          return (
            <FieldRow
              key={f.key}
              id={id}
              data-search-id={`process:${p.id ?? ""}:${f.key}`}
              label={f.label}
              unit={f.unit}
              help={f.help}
            >
              {f.unit ? (
                <UnitField unit={f.unit}>{inputs[f.key](id)}</UnitField>
              ) : (
                inputs[f.key](id)
              )}
            </FieldRow>
          );
        })}
      </div>
      <div className="dialog-foot">
        <button type="button" className="btn btn-primary" onClick={onClose}>
          Готово
        </button>
      </div>
    </dialog>
  );
}

export function Processes() {
  const route = useProjectRoute();
  const projectId = route.projectId;

  const projectQ = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => fetchProject(projectId as string),
    enabled: Boolean(projectId),
  });

  const [error, setError] = useState("");
  // editing is the process open in the dialog; the others in the project see it and its card is outlined there.
  const [editing, setEditing] = useState<string | null>(null);

  const store = useProjectStore(route.storeKey);
  const [history] = useUndoHistory(route.storeKey);
  const selectProcesses = useMemo(() => processesSelector(), []);
  const items = useStoreSelector(store, selectProcesses);
  const ready = useStoreSelector(store, () => store.ready);

  // A process field found by the search opens in its dialog once the user confirms it; while the user only
  // looks, the search shows the card.
  useOnSearchTarget((t) => {
    if (t.phase === "confirmed" && t.entry.kind === "processField" && t.entry.anchor) {
      setEditing(t.entry.anchor.replace(/^process:/, ""));
    }
  });

  const others = usePresence(route.storeKey);
  useReportSelection(route.storeKey, editing ? { coll: "processes", id: editing } : null);
  const watchers = useMemo(() => {
    const out = new Map<string, { initials: string; color: number }>();
    for (const person of others) {
      for (const sel of person.selections) {
        if (sel?.coll === "processes") {
          out.set(sel.id, { initials: person.initials, color: person.color });
        }
      }
    }
    return out;
  }, [others]);

  // run sends one edit as one transaction and records it for ctrl+Z; it shows why the schema refused it.
  function run(edit: Edit): boolean {
    if (edit.ops.length === 0) {
      return true;
    }
    const r = history.run(edit.label, edit.ops);
    setError(
      r.outcome.status === "rejected"
        ? `Изменение не применено. ${reasonText(r.outcome.reason)}`
        : "",
    );
    return r.outcome.status === "applied";
  }

  // update writes one field as its own transaction, so two people editing different processes never overwrite
  // each other.
  function update(i: number, patch: ProcessPatch) {
    const item = items[i];
    if (item.id) {
      run(processFieldEdit(store.getState(), item.id, patch));
    }
  }

  function remove(i: number) {
    const item = items[i];
    if (item.id) {
      run(deleteProcessEdit(item.id));
    }
  }

  function add(p: Process) {
    if (run(addProcessEdit(store.getState(), p)) && p.id) {
      setEditing(p.id);
    }
  }

  function copyOf(p: Process): Process {
    return {
      ...structuredClone(p),
      id: uuid(),
      code: freeCode(items),
      name: `${p.name || p.code} (копия)`,
      sort_order: items.length,
    };
  }

  if ((projectId && projectQ.isPending) || !ready) {
    return (
      <section>
        <h1 className="sr-only">Процессы</h1>
        <p>Загрузка...</p>
      </section>
    );
  }

  const editIndex = items.findIndex((p) => p.id === editing);

  return (
    <section>
      <h1 className="sr-only">Базовые процессы</h1>
      {error ? (
        <p className="error">
          <Reflow>{error}</Reflow>
        </p>
      ) : null}

      <ul className="project-cards">
        {items.map((p, i) => {
          const key = p.id ?? `new-${i}`;
          const watcher = p.id ? watchers.get(p.id) : undefined;
          return (
            <li
              key={key}
              className={watcher ? "task-card project-card is-watched" : "task-card project-card"}
              data-process={p.code}
              data-search-id={p.id ? `process:${p.id}` : undefined}
              style={
                watcher
                  ? ({ "--watch-color": `var(--presence-${watcher.color + 1})` } as CSSProperties)
                  : undefined
              }
            >
              <div className="task-card-head">
                <button
                  type="button"
                  className="task-card-title"
                  onClick={() => setEditing(p.id ?? null)}
                >
                  {p.name || p.code}
                </button>
                {watcher ? (
                  <span className="face" title={`Правит ${watcher.initials}`} aria-hidden="true">
                    {watcher.initials}
                  </span>
                ) : p.is_baseline ? (
                  <span />
                ) : (
                  <span className="tag">не в базе</span>
                )}
                <span className="task-card-summary">
                  {processLines(p).map((line) => (
                    <span key={line} className="task-card-line">
                      <Reflow>{line}</Reflow>
                    </span>
                  ))}
                </span>
              </div>
              <div className="project-card-actions">
                <button type="button" onClick={() => add(copyOf(p))}>
                  Копия
                </button>
                <button
                  type="button"
                  className="btn btn-danger"
                  onClick={() => remove(i)}
                  disabled={items.length <= 1}
                >
                  Удалить
                </button>
              </div>
            </li>
          );
        })}
      </ul>

      {editIndex >= 0 ? (
        <ProcessDialog
          key={editing}
          p={items[editIndex]}
          onClose={() => setEditing(null)}
          update={(patch) => update(editIndex, patch)}
        />
      ) : null}

      <ActionBar label="Действия с процессами">
        <button
          type="button"
          className="btn btn-primary"
          data-search-id="action:add-process"
          onClick={() =>
            add({
              id: uuid(),
              code: freeCode(items),
              name: "Новый процесс",
              task_type: "pallet_move",
              is_baseline: true,
              demand: { units_per_day: 0, unit: "ед" },
              sla: { max_wait_min: 30, max_cycle_min: 40 },
              durations: { load_s: 60, unload_s: 60, travel_s: 120 },
              baseline_staff: { headcount: 1, role: "" },
              sort_order: items.length,
            })
          }
        >
          Добавить процесс
        </button>
      </ActionBar>
    </section>
  );
}

import { use, useCallback, useEffect, useMemo, useRef, useState } from "react";

import type {
  ObjectSchema,
  ObjectType,
  ParamValue,
  ParamsMap,
  Project,
  SchemaField,
} from "../api/client";
import { plural } from "../fleet/format";
import { defaultsFromSchema, fieldList, sanitizeParams } from "../guest/store";
import { ReportTabErrors } from "../layout/shellContext";
import { checkParams } from "../params/check";
import { importParamsFile, maxImportBytes, type ImportResult } from "../params/importParams";
import { downloadParamsTemplate } from "../params/template";
import { paramFieldEdit, paramsEdit, paramsOf } from "../projects/econRecords";
import { reasonText } from "../store/reasons";
import { useProjectStore, useStoreSelector, useUndoHistory } from "../store/useProjectStore";
import { undoKeys } from "../store/useUndoShortcuts";
import { ActionBar } from "../ui/ActionBar";
import { ChevronDownIcon } from "../ui/icons";
import { Popover } from "../ui/Popover";
import { Reflow } from "../ui/Reflow";
import { followStore } from "./followStore";
import { ParamGrid } from "./ParamGrid";

type Props = {
  objectType: ObjectType;
  schema: ObjectSchema;
  tab: string;
  storeKey: string;
  project: Project | undefined;
};

function savedValues(
  schema: ObjectSchema,
  project: Project | undefined,
  stored: ParamsMap,
): ParamsMap {
  const list = fieldList(schema);
  if (project && project.params && typeof project.params === "object") {
    return sanitizeParams(list, project.params);
  }
  return sanitizeParams(list, stored);
}

type Tools = {
  busy: boolean;
  status?: string;
  onDemo: () => void;
  onTemplate: (format: "xlsx" | "csv") => void;
  onImport: () => void;
};

// ObjectTools act on every tab of the form at once. They sit in the action bar; below 1280 px they fold into one
// menu, so the status line keeps its room.
function ObjectTools({ busy, status, onDemo, onTemplate, onImport }: Tools) {
  const [open, setOpen] = useState(false);
  const act = (fn: () => void) => () => {
    setOpen(false);
    fn();
  };
  return (
    <ActionBar label="Действия с параметрами" status={status}>
      <div className="tools-wide">
        <button
          type="button"
          className="btn btn-text"
          data-search-id="action:import"
          onClick={onImport}
          disabled={busy}
        >
          Импорт из Excel
        </button>
        <button
          type="button"
          className="btn btn-text"
          onClick={() => onTemplate("xlsx")}
          disabled={busy}
        >
          Шаблон XLSX
        </button>
        <button
          type="button"
          className="btn btn-text"
          onClick={() => onTemplate("csv")}
          disabled={busy}
        >
          Шаблон CSV
        </button>
        <button type="button" className="btn btn-text" onClick={onDemo} disabled={busy}>
          Демо-значения
        </button>
      </div>
      <Popover
        open={open}
        onOpenChange={setOpen}
        label="Действия с параметрами"
        align="end"
        side="above"
        className="tools-narrow"
        trigger={(t) => (
          <button type="button" className="btn btn-text" {...t}>
            Действия
            <ChevronDownIcon size={16} />
          </button>
        )}
      >
        <button type="button" className="menu-item" onClick={act(onImport)} disabled={busy}>
          Импорт из Excel или CSV
        </button>
        <button
          type="button"
          className="menu-item"
          onClick={act(() => onTemplate("xlsx"))}
          disabled={busy}
        >
          Скачать шаблон XLSX
        </button>
        <button
          type="button"
          className="menu-item"
          onClick={act(() => onTemplate("csv"))}
          disabled={busy}
        >
          Скачать шаблон CSV
        </button>
        <button type="button" className="menu-item" onClick={act(onDemo)} disabled={busy}>
          Вернуть демо-значения
        </button>
      </Popover>
    </ActionBar>
  );
}

export function ObjectForm({ objectType, schema, tab, storeKey, project }: Props) {
  const fields = useMemo(() => fieldList(schema), [schema]);
  const store = useProjectStore(storeKey);
  const [history] = useUndoHistory(storeKey);
  const storedParams = useStoreSelector(store, paramsOf);
  const reportErrors = use(ReportTabErrors);
  // values is what the fields show; the store holds what is saved. A field follows the store unless it is
  // focused or holds a draft that is not saved yet (pending), so an undo or a co-author's edit shows up.
  const [values, setValues] = useState<ParamsMap>(() => savedValues(schema, project, storedParams));
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [note, setNote] = useState("");
  const [importReport, setImportReport] = useState<ImportResult | null>(null);
  const [importing, setImporting] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  // A field saves what is on screen when its timer runs out, which is after the render that set it, so the
  // form keeps the current values where an event handler can reach them.
  const latest = useRef(values);
  const pending = useRef(new Set<string>());
  const focused = useRef<string | null>(null);

  const groups = schema.groups.filter((g) => g.tab === tab);
  const tabErrors = groups.reduce(
    (n, g) => n + g.fields.filter((f) => fieldErrors[f.id]).length,
    0,
  );
  const tabStatus =
    tabErrors > 0
      ? `${tabErrors} ${plural(tabErrors, "поле с ошибкой", "поля с ошибкой", "полей с ошибкой")}: такое значение не сохраняется`
      : undefined;
  const tabLabel = schema.tabs.find((t) => t.id === tab)?.label ?? "";

  // The subtab row marks the tabs whose fields hold a value that failed a check.
  useEffect(() => {
    const bad = new Set<string>();
    for (const g of schema.groups) {
      if (g.fields.some((f) => fieldErrors[f.id])) {
        bad.add(g.tab);
      }
    }
    reportErrors({ key: storeKey, tabs: [...bad] });
  }, [fieldErrors, schema, reportErrors, storeKey]);
  useEffect(() => () => reportErrors({ key: storeKey, tabs: [] }), [reportErrors, storeKey]);

  const follow = useCallback(() => {
    const busy = new Set(pending.current);
    if (focused.current) {
      busy.add(focused.current);
    }
    const next = followStore(fields, latest.current, paramsOf(store.getState()), busy);
    if (next !== latest.current) {
      latest.current = next;
      setValues(next);
    }
  }, [fields, store]);
  useEffect(() => follow(), [follow, storedParams]);

  function leaveField(f: SchemaField) {
    if (focused.current === f.id) {
      focused.current = null;
    }
    follow();
  }

  function setField(f: SchemaField, raw: ParamValue | undefined) {
    setNote("");
    const next = { ...latest.current };
    if (raw === undefined) {
      delete next[f.id];
    } else {
      next[f.id] = raw;
    }
    latest.current = next;
    pending.current.add(f.id);
    setValues(next);
  }

  function markField(id: string, message: string | undefined) {
    setFieldErrors((prev) => {
      if ((prev[id] ?? undefined) === message) {
        return prev;
      }
      const next = { ...prev };
      if (message === undefined) {
        delete next[id];
      } else {
        next[id] = message;
      }
      return next;
    });
  }

  // commitField saves one parameter as its own transaction, so a parameter someone else is editing keeps their
  // value. The operation schema leaves the ranges of an object type to the server, so the form checks them
  // here: a value out of range stays in the field with its error and is not sent.
  function commitField(f: SchemaField) {
    const current = latest.current;
    const bad = checkParams([f], current)[f.id];
    if (bad) {
      markField(f.id, bad);
      return;
    }
    markField(f.id, undefined);
    setFormError("");
    const edit = paramFieldEdit(store.getState(), f.id, current[f.id]);
    if (edit.ops.length > 0) {
      const r = history.run(edit.label, edit.ops);
      if (r.outcome.status === "rejected") {
        markField(f.id, reasonText(r.outcome.reason));
        return;
      }
    }
    pending.current.delete(f.id);
  }

  // writeAll saves a whole set of parameters at once: the demo values, or the cells an imported file filled.
  // It is one action, so it is one transaction, and ctrl+Z takes all of it back.
  function writeAll(next: ParamsMap, label: string): boolean {
    setValues(next);
    latest.current = next;
    const bad = checkParams(fields, next);
    if (Object.keys(bad).length > 0) {
      setFormError("Параметры не прошли проверку. Исправьте поля, отмеченные в подвкладках.");
      setFieldErrors(bad);
      pending.current = new Set(Object.keys(bad));
      follow();
      return false;
    }
    setFieldErrors({});
    const edit = paramsEdit(store.getState(), next);
    if (edit.ops.length > 0) {
      const r = history.run(label, edit.ops);
      if (r.outcome.status === "rejected") {
        setFormError(`Изменение не применено. ${reasonText(r.outcome.reason)}`);
        const field = r.outcome.details.field;
        setFieldErrors(field ? { [field]: reasonText(r.outcome.reason) } : {});
        pending.current = new Set(field ? [field] : []);
        follow();
        return false;
      }
    }
    pending.current.clear();
    return true;
  }

  function onDemo() {
    setFormError("");
    setImportReport(null);
    if (writeAll(defaultsFromSchema(fields), "Вернуть демо-значения")) {
      setNote(`Подставлены демо-значения из схемы. ${undoKeys} вернёт прежние.`);
    }
  }

  async function onTemplate(format: "xlsx" | "csv") {
    setFormError("");
    try {
      await downloadParamsTemplate({ format, objectType, sheetName: schema.label, fields, values });
    } catch {
      setFormError("Не удалось скачать шаблон. Повторите попытку.");
    }
  }

  async function onImportFile(file: File | undefined) {
    if (fileRef.current) {
      fileRef.current.value = "";
    }
    if (!file) {
      return;
    }
    setFormError("");
    setFieldErrors({});
    setNote("");
    if (file.size > maxImportBytes) {
      setFormError("Файл больше 2 МБ. Уменьшите файл или загрузите csv-шаблон.");
      return;
    }
    setImporting(true);
    try {
      const data = await file.arrayBuffer();
      const out = await importParamsFile({
        fields: fields.map((f) => ({
          id: f.id,
          label: f.label,
          type: f.type,
          min: f.min,
          max: f.max,
          allow_unknown: f.allow_unknown,
          options: f.options,
          aliases: f.aliases,
        })),
        objectType,
        filename: file.name,
        data,
      });
      setImportReport(out);
      const bad: Record<string, string> = {};
      for (const iss of out.issues) {
        if (iss.field && iss.level === "error") {
          bad[iss.field] = iss.message;
        }
      }
      // A cell the file got wrong is not written; the rest of the file is, in one go.
      if (
        writeAll({ ...latest.current, ...out.values }, "Загрузить параметры из файла") &&
        Object.keys(bad).length > 0
      ) {
        setFieldErrors(bad);
      }
    } catch {
      setImportReport(null);
      setFormError(
        "Не удалось прочитать файл. Используйте xlsx или csv с колонками Параметр и Базовое значение, либо скачайте шаблон.",
      );
    } finally {
      setImporting(false);
    }
  }

  return (
    <section className="object-page">
      <h1 className="sr-only">{tabLabel}</h1>
      <input
        ref={fileRef}
        className="file-input"
        type="file"
        accept=".xlsx,.xls,.csv,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
        onChange={(e) => void onImportFile(e.target.files?.[0] ?? undefined)}
      />
      <ObjectTools
        busy={importing}
        status={tabStatus}
        onDemo={onDemo}
        onTemplate={(f) => void onTemplate(f)}
        onImport={() => fileRef.current?.click()}
      />

      {formError ? (
        <p className="error">
          <Reflow>{formError}</Reflow>
        </p>
      ) : null}
      {note ? (
        <p>
          <Reflow>{note}</Reflow>
        </p>
      ) : null}

      {importReport ? (
        <div className="import-report">
          <p>
            <Reflow>
              Импортировано {importReport.importedCount} из {importReport.fieldCount} полей
              {importReport.sheet ? ` (лист ${importReport.sheet})` : ""}.
              {importReport.issues.some((i) => i.level === "error")
                ? " Ячейки с ошибкой не подставлены: исправьте их в форме вручную или в файле и загрузите снова."
                : importReport.issues.length > 0
                  ? " Полей нет в файле: в форме остались прежние значения, их можно изменить вручную."
                  : " Все поля из схемы найдены в файле."}
            </Reflow>
          </p>
          {importReport.issues.length > 0 ? (
            <ul>
              {importReport.issues.map((iss, idx) => (
                <li
                  key={`${iss.field ?? ""}:${iss.row ?? ""}:${idx}`}
                  className={iss.level === "error" ? "error" : undefined}
                >
                  <Reflow>{iss.message}</Reflow>
                </li>
              ))}
            </ul>
          ) : null}
          <div className="actions">
            <button type="button" className="btn btn-text" onClick={() => setImportReport(null)}>
              Скрыть отчёт
            </button>
          </div>
        </div>
      ) : null}

      <ParamGrid
        groups={groups}
        values={values}
        errors={fieldErrors}
        onChange={setField}
        onCommit={commitField}
        onEnter={(f) => (focused.current = f.id)}
        onLeave={leaveField}
      />
    </section>
  );
}

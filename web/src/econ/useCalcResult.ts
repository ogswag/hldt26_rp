import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useMemo } from "react";

import {
  ApiError,
  calculateProject,
  fetchProject,
  type CalculateResult,
  type EconOverrides,
  type FieldError,
  type ObjectType,
  type ParamsMap,
} from "../api/client";
import { calculateOrLocal, type Preliminary } from "../engine/fallback";
import { currentModelVersion } from "./modelVersion";
import {
  calcFingerprint,
  isObjectType,
  loadGuestCalc,
  overridesForRequest,
  saveGuestCalc,
} from "../guest/store";
import {
  objectTypeOf,
  overridesEdit,
  overridesOf,
  paramsOf,
  selectedSolutions,
} from "../projects/econRecords";
import { equal, type State } from "../store/apply";
import { stateFingerprint } from "../store/fingerprint";
import type { TabId } from "../layout/nav";
import { useProjectRoute } from "../layout/route";
import { useProjectStore, useStoreSelector, useUndoHistory } from "../store/useProjectStore";

export type CalcBundle = {
  objectType: ObjectType | null;
  projectId: string | undefined;
  result: Preliminary<CalculateResult> | null;
  loading: boolean;
  preliminary: boolean;
  error: string;
  details: FieldError[];
  needType: boolean;
  recalc: (overrides?: EconOverrides) => Promise<void>;
  href: (tab: TabId, sub?: string) => string;
  storedOverrides: EconOverrides;
  params: ParamsMap;
  // includeIds is the robots the project has picked; the calculation and the export both go by it.
  includeIds: string[];
  stale: boolean;
};

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}

// stateKey ties a demo's result to its records, so an edit of a process, a variant or a fleet is recalculated.
function stateKey(fingerprint: string, collections: State | undefined): string {
  return collections ? `${fingerprint}#${stateFingerprint(collections)}` : fingerprint;
}

function errorDetails(err: unknown): FieldError[] {
  if (err instanceof ApiError) {
    return err.details;
  }
  return [];
}

export function useCalcResult(): CalcBundle {
  const route = useProjectRoute();
  const projectId = route.projectId;
  const queryClient = useQueryClient();

  const projectQ = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => fetchProject(projectId as string),
    enabled: Boolean(projectId),
  });

  const store = useProjectStore(route.storeKey);
  const [history] = useUndoHistory(route.storeKey);
  const storeReady = useStoreSelector(store, () => store.ready);
  const storedType = useStoreSelector(store, objectTypeOf);
  const storedParams = useStoreSelector(store, paramsOf);
  const storedSelected = useStoreSelector(store, selectedSolutions, equal);
  const storedOverrides = useStoreSelector(store, overridesOf, equal);
  const demoFingerprint = useStoreSelector(store, () => (projectId ? "" : stateFingerprint(store.getState())));

  const objectType: ObjectType | null = useMemo(() => {
    if (projectQ.data && isObjectType(projectQ.data.object_type)) {
      return projectQ.data.object_type;
    }
    return route.demo ?? storedType;
  }, [projectQ.data, route.demo, storedType]);

  const params: ParamsMap = useMemo(() => {
    if (projectQ.data?.params && typeof projectQ.data.params === "object") {
      return projectQ.data.params;
    }
    return storedParams;
  }, [projectQ.data, storedParams]);

  const includeIds = useMemo(() => {
    if (projectId) {
      return projectQ.data?.match_selected_ids ?? [];
    }
    return storedSelected;
  }, [projectId, projectQ.data, storedSelected]);

  const fingerprint =
    objectType !== null
      ? calcFingerprint(objectType, params, includeIds, storedOverrides) + (projectId ? "" : `#${demoFingerprint}`)
      : "";
  const projectPending = Boolean(projectId) && projectQ.isPending;

  const ready = objectType !== null && !projectPending && !projectQ.isError && storeReady;

  const calcQ = useQuery({
    queryKey: ["calc", projectId ?? "guest", objectType, fingerprint],
    enabled: ready,
    // A changed input keeps the old numbers on screen until the new ones arrive, so the page never blanks.
    placeholderData: keepPreviousData,
    queryFn: async () => {
      if (projectId) {
        const existing = projectQ.data?.results;
        if (existing && existing.model_version === currentModelVersion && Array.isArray(existing.scenarios) && existing.scenarios.length > 0) {
          return existing;
        }
        const out = await calculateProject(projectId, {
          include_ids: includeIds,
          overrides: overridesForRequest(storedOverrides),
        });
        void queryClient.invalidateQueries({ queryKey: ["project", projectId] });
        return out;
      }
      const type = objectType as ObjectType;
      const ov = storedOverrides;
      const collections = store.getState();
      const fp = stateKey(calcFingerprint(type, params, includeIds, ov), collections);
      const cached = loadGuestCalc(type, fp);
      if (cached) {
        return cached;
      }
      const out = await calculateOrLocal({
        object_type: type,
        params,
        include_ids: includeIds,
        overrides: overridesForRequest(ov),
        collections,
      });
      // A preliminary result is not cached: the next visit should ask the server again.
      if (!out.preliminary) {
        saveGuestCalc(type, fp, out);
      }
      return out;
    },
  });

  const mut = useMutation({
    mutationFn: async (overrides: EconOverrides | undefined) => {
      if (!objectType) {
        throw new Error("Сначала выберите тип объекта.");
      }
      if (projectId) {
        // What-if belongs to the project: it is written as a transaction, so the others see it and ctrl+Z
        // takes it back. The run itself is always computed by the server.
        if (overrides !== undefined) {
          const edit = overridesEdit(store.getState(), overrides);
          if (edit.ops.length > 0) {
            history.run(edit.label, edit.ops);
          }
        }
        // The server calculates its own copy of the project, so the edits have to reach it first.
        await store.whenSent(5000);
        return calculateProject(projectId, {
          include_ids: includeIds,
          overrides: overridesForRequest(overrides),
        });
      }
      // What-if belongs to the guest project too: it is written as a transaction, so ctrl+Z takes it back.
      const ov = overrides ?? storedOverrides;
      if (overrides !== undefined) {
        const edit = overridesEdit(store.getState(), ov);
        if (edit.ops.length > 0) {
          history.run(edit.label, edit.ops);
        }
      }
      const collections = store.getState();
      const fp = stateKey(calcFingerprint(objectType, params, includeIds, ov), collections);
      const out = await calculateOrLocal({
        object_type: objectType,
        params,
        include_ids: includeIds,
        overrides: overridesForRequest(ov),
        collections,
      });
      if (!out.preliminary) {
        saveGuestCalc(objectType, fp, out);
      }
      return out;
    },
    onSuccess: (out, overrides) => {
      if (!objectType) {
        return;
      }
      const ov = overrides ?? storedOverrides;
      const fp = stateKey(calcFingerprint(objectType, params, includeIds, ov ?? {}), projectId ? undefined : store.getState());
      queryClient.setQueryData(["calc", projectId ?? "guest", objectType, fp], out);
      if (projectId) {
        void queryClient.invalidateQueries({ queryKey: ["project", projectId] });
      }
    },
  });

  const recalc = useCallback(
    async (overrides?: EconOverrides) => {
      await mut.mutateAsync(overrides);
    },
    [mut],
  );

  const href = route.href;

  const loadError =
    (projectQ.isError ? errorMessage(projectQ.error, "Не удалось загрузить проект.") : "") ||
    (calcQ.isError ? errorMessage(calcQ.error, "Не удалось выполнить расчёт.") : "") ||
    (mut.isError ? errorMessage(mut.error, "Не удалось выполнить расчёт.") : "");

  const projectDetails = errorDetails(projectQ.error);
  const calcDetails = mut.isError ? errorDetails(mut.error) : errorDetails(calcQ.error);
  const mergedDetails = projectDetails.length > 0 ? projectDetails : calcDetails;

  const result: Preliminary<CalculateResult> | null = mut.data ?? calcQ.data ?? null;

  return {
    objectType,
    projectId,
    result,
    preliminary: Boolean(result?.preliminary),
    loading:
      projectPending ||
      !storeReady ||
      (ready && (calcQ.isPending || (calcQ.isPlaceholderData && calcQ.isFetching))) ||
      mut.isPending,
    error: loadError,
    details: mergedDetails,
    needType: !objectType && !projectPending && storeReady,
    recalc,
    href,
    storedOverrides,
    includeIds,
    params,
    stale: Boolean(projectQ.data?.stale),
  };
}

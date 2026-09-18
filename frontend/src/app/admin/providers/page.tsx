"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  deleteModel,
  deleteProvider,
  listModels,
  listProviders,
  type ModelListItem,
  type Provider,
} from "@/lib/api";
import CreateModelForm from "@/components/CreateModelForm";
import CreateProviderForm from "@/components/CreateProviderForm";
import EditModelForm from "@/components/EditModelForm";
import EditProviderForm from "@/components/EditProviderForm";
import Modal from "@/components/Modal";
import Pagination from "@/components/Pagination";
import SearchInput from "@/components/SearchInput";
import StatusBadge from "@/components/StatusBadge";
import { usePaginatedFilter } from "@/hooks/usePaginatedFilter";
import { isTestProviderName } from "@/lib/testProviders";

const GRID = "grid grid-cols-[24px_1.4fr_2fr_0.8fr_0.7fr_0.7fr_auto] gap-2";

function isTestProvider(provider: Provider) {
  return isTestProviderName(provider.name);
}

export default function ProvidersPage() {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [models, setModels] = useState<ModelListItem[]>([]);
  const [loadingProviders, setLoadingProviders] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [editingProviderId, setEditingProviderId] = useState<string | null>(null);
  const [editingModelId, setEditingModelId] = useState<string | null>(null);
  const [addingModelFor, setAddingModelFor] = useState<string | null>(null);
  const [showTests, setShowTests] = useState(false);

  function refreshAll() {
    setLoadingProviders(true);
    Promise.all([listProviders(), listModels()])
      .then(([p, m]) => {
        setProviders(p);
        setModels(m);
      })
      .catch(() => {
        setProviders([]);
        setModels([]);
      })
      .finally(() => setLoadingProviders(false));
  }

  useEffect(() => {
    refreshAll();
  }, []);

  const liveProviders = useMemo(() => providers.filter((p) => !isTestProvider(p)), [providers]);
  const testProviders = useMemo(() => providers.filter(isTestProvider), [providers]);

  const filterFn = useCallback(
    (provider: Provider, query: string) => provider.name.toLowerCase().includes(query),
    [],
  );

  const { search, setSearch, page, setPage, totalPages, pageItems, totalCount } = usePaginatedFilter(
    liveProviders,
    filterFn,
  );

  function handleCreated() {
    setModalOpen(false);
    refreshAll();
  }

  function toggleExpand(id: string) {
    setExpandedId((prev) => (prev === id ? null : id));
    setEditingProviderId(null);
    setEditingModelId(null);
    setAddingModelFor(null);
  }

  async function handleDeleteProvider(id: string) {
    if (!confirm("Delete this provider and all of its models? This cannot be undone.")) return;
    await deleteProvider(id);
    refreshAll();
  }

  async function handleDeleteModel(id: string) {
    if (!confirm("Delete this model? Users granted access to it will lose access.")) return;
    await deleteModel(id);
    refreshAll();
  }

  const headerRow = (
    <div className={`${GRID} text-left text-gray-400 border-b border-gray-100 py-2 font-medium`}>
      <span></span>
      <span>Name</span>
      <span>Endpoint</span>
      <span>Method</span>
      <span>Max RPS</span>
      <span>Max/hr</span>
      <span></span>
    </div>
  );

  function renderProviderRow(provider: Provider) {
    const isExpanded = expandedId === provider.id;
    const providerModels = models.filter((m) => m.provider_id === provider.id);
    return (
      <div key={provider.id} className="border-b border-gray-50 last:border-0">
        <div
          className={`${GRID} items-center py-2.5 hover:bg-gray-50 cursor-pointer`}
          onClick={() => toggleExpand(provider.id)}
        >
          <button
            type="button"
            aria-label={isExpanded ? "Collapse" : "Expand"}
            className={`text-gray-400 transition-transform ${isExpanded ? "rotate-90" : ""}`}
          >
            &gt;
          </button>
          <span>{provider.name}</span>
          <span className="truncate max-w-xs">{provider.endpoint_url}</span>
          <span>{provider.http_method}</span>
          <span>{provider.max_outbound_rps}</span>
          <span>{provider.max_calls_per_hour ?? "unlimited"}</span>
          <span className="text-right">
            {providerModels.length} model{providerModels.length === 1 ? "" : "s"}
          </span>
        </div>

        {isExpanded && (
          <div className="pl-8 pb-3 pr-2 space-y-3" onClick={(e) => e.stopPropagation()}>
            {editingProviderId === provider.id ? (
              <EditProviderForm
                provider={provider}
                onSaved={() => {
                  setEditingProviderId(null);
                  refreshAll();
                }}
                onCancel={() => setEditingProviderId(null)}
              />
            ) : (
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => setEditingProviderId(provider.id)}
                  className="rounded-md border border-gray-300 px-2.5 py-1 text-xs font-medium hover:bg-gray-50"
                >
                  Edit provider
                </button>
                <button
                  type="button"
                  onClick={() => handleDeleteProvider(provider.id)}
                  className="rounded-md border border-gray-300 px-2.5 py-1 text-xs font-medium text-red-600 hover:bg-red-50"
                >
                  Delete provider
                </button>
              </div>
            )}

            <div className="border-t border-gray-100 pt-2.5">
              <div className="flex items-center justify-between mb-2">
                <p className="text-xs font-medium">Models</p>
                <button
                  type="button"
                  onClick={() => setAddingModelFor(provider.id)}
                  className="text-xs text-gray-500 hover:text-gray-700 underline"
                >
                  + Add model
                </button>
              </div>

              <div className="space-y-2">
                {providerModels.length === 0 && (
                  <p className="text-xs text-gray-400">No models yet.</p>
                )}
                {providerModels.map((model) =>
                  editingModelId === model.id ? (
                    <EditModelForm
                      key={model.id}
                      model={model}
                      onSaved={() => {
                        setEditingModelId(null);
                        refreshAll();
                      }}
                      onCancel={() => setEditingModelId(null)}
                    />
                  ) : (
                    <div
                      key={model.id}
                      className="flex items-center justify-between rounded-md border border-gray-200 px-2.5 py-1.5"
                    >
                      <div>
                        <p className="font-medium">{model.name}</p>
                        <p className="text-[11px] text-gray-400 font-mono">{model.resolved_model}</p>
                      </div>
                      <div className="flex items-center gap-3">
                        <span className="text-[11px] text-gray-400">
                          ${model.input_price_per_1k_tokens}/${model.output_price_per_1k_tokens} per 1k tokens
                        </span>
                        <StatusBadge status={model.blocked ? "blocked" : "active"} />
                        <button
                          type="button"
                          onClick={() => setEditingModelId(model.id)}
                          className="text-xs text-gray-500 hover:text-gray-700 underline"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          onClick={() => handleDeleteModel(model.id)}
                          className="text-xs text-red-600 hover:text-red-700 underline"
                        >
                          Delete
                        </button>
                      </div>
                    </div>
                  ),
                )}
              </div>

              {addingModelFor === provider.id && (
                <div className="mt-2 rounded-md border border-gray-200 p-2.5">
                  <CreateModelForm
                    providers={[provider]}
                    onCreated={() => {
                      setAddingModelFor(null);
                      refreshAll();
                    }}
                  />
                  <button
                    type="button"
                    onClick={() => setAddingModelFor(null)}
                    className="mt-2 text-xs text-gray-400 hover:text-gray-600 underline"
                  >
                    Cancel
                  </button>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    );
  }

  return (
    <>
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-lg font-serif">Providers</h1>
        <button
          type="button"
          onClick={() => setModalOpen(true)}
          className="bg-black text-white rounded-md px-4 py-1.5 text-xs font-medium"
        >
          + Add provider
        </button>
      </div>

      <div className="bg-white rounded-lg shadow-sm p-4">
        <div className="flex items-center justify-between mb-3">
          <p className="text-xs text-gray-400">
            {totalCount} provider{totalCount === 1 ? "" : "s"}
          </p>
          <SearchInput value={search} onChange={setSearch} placeholder="Search providers..." />
        </div>

        {loadingProviders ? (
          <p className="text-xs text-gray-400">Loading...</p>
        ) : pageItems.length === 0 ? (
          <p className="text-xs text-gray-400">No providers found</p>
        ) : (
          <div className="text-xs">
            {headerRow}
            {pageItems.map(renderProviderRow)}
          </div>
        )}

        <Pagination page={page} totalPages={totalPages} onChange={setPage} />
      </div>

      {!loadingProviders && testProviders.length > 0 && (
        <div className="mt-4">
          <button
            type="button"
            onClick={() => setShowTests((prev) => !prev)}
            aria-expanded={showTests}
            className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-xs font-medium hover:bg-gray-50"
          >
            {showTests ? "Hide test providers" : "Test"} ({testProviders.length})
          </button>

          {showTests && (
            <div className="bg-white rounded-lg shadow-sm p-4 mt-2">
              <p className="text-xs text-gray-400 mb-3">
                Dummy providers pointed at a local test server — not real upstreams.
              </p>
              <div className="text-xs">
                {headerRow}
                {testProviders.map(renderProviderRow)}
              </div>
            </div>
          )}
        </div>
      )}

      {modalOpen && (
        <Modal title="Add provider" size="lg" onClose={() => setModalOpen(false)}>
          <CreateProviderForm onCreated={handleCreated} />
        </Modal>
      )}
    </>
  );
}

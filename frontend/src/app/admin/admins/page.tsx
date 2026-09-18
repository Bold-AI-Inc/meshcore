"use client";

import { useCallback, useEffect, useState } from "react";
import {
  deleteAdmin,
  listAdmins,
  pauseAdmin,
  resumeAdmin,
  type AdminListItem,
} from "@/lib/api";
import CreateAdminForm from "@/components/CreateAdminForm";
import Modal from "@/components/Modal";
import Pagination from "@/components/Pagination";
import SearchInput from "@/components/SearchInput";
import StatusBadge from "@/components/StatusBadge";
import { usePaginatedFilter } from "@/hooks/usePaginatedFilter";
import { useAdminSession } from "@/context/AdminSessionContext";

export default function AdminsPage() {
  const { email: currentEmail, isSuperAdmin } = useAdminSession();
  const [admins, setAdmins] = useState<AdminListItem[]>([]);
  const [loadingAdmins, setLoadingAdmins] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<AdminListItem | null>(null);
  const [error, setError] = useState("");

  function refreshAdmins() {
    setLoadingAdmins(true);
    listAdmins()
      .then(setAdmins)
      .catch(() => setAdmins([]))
      .finally(() => setLoadingAdmins(false));
  }

  useEffect(() => {
    refreshAdmins();
  }, []);

  const filterFn = useCallback(
    (admin: AdminListItem, query: string) => admin.email.toLowerCase().includes(query),
    [],
  );

  const { search, setSearch, page, setPage, totalPages, pageItems, totalCount } = usePaginatedFilter(
    admins,
    filterFn,
  );

  function handleCreated() {
    setModalOpen(false);
    refreshAdmins();
  }

  async function runAction(id: string, action: () => Promise<unknown>) {
    setBusyId(id);
    setError("");
    try {
      await action();
      refreshAdmins();
    } catch {
      setError("That action could not be completed.");
    } finally {
      setBusyId(null);
      setConfirmDelete(null);
    }
  }

  return (
    <>
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-lg font-serif">Admins</h1>
        <button
          type="button"
          onClick={() => setModalOpen(true)}
          className="bg-black text-white rounded-md px-4 py-1.5 text-xs font-medium"
        >
          + Add admin
        </button>
      </div>

      {error && <p className="text-xs text-red-600 mb-3">{error}</p>}

      <div className="bg-white rounded-lg shadow-sm p-4">
        <div className="flex items-center justify-between mb-3">
          <p className="text-xs text-gray-400">
            {totalCount} admin{totalCount === 1 ? "" : "s"}
          </p>
          <SearchInput value={search} onChange={setSearch} placeholder="Search admins..." />
        </div>

        {loadingAdmins ? (
          <p className="text-xs text-gray-400">Loading...</p>
        ) : pageItems.length === 0 ? (
          <p className="text-xs text-gray-400">No admins found</p>
        ) : (
          <table className="w-full text-xs">
            <thead>
              <tr className="text-left text-gray-400 border-b border-gray-100">
                <th className="py-2 font-medium">Email</th>
                <th className="py-2 font-medium">Status</th>
                <th className="py-2 font-medium">Created by</th>
                {isSuperAdmin && <th className="py-2 font-medium text-right">Actions</th>}
              </tr>
            </thead>
            <tbody>
              {pageItems.map((admin) => {
                const isSelf = admin.email.toLowerCase() === currentEmail.toLowerCase();
                const busy = busyId === admin.id;
                return (
                  <tr key={admin.id} className="border-b border-gray-50 last:border-0 hover:bg-gray-50">
                    <td className="py-2.5">
                      {admin.email}
                      {isSelf && <span className="text-gray-400 ml-1.5">(you)</span>}
                    </td>
                    <td className="py-2.5">
                      <StatusBadge status={admin.status} />
                    </td>
                    <td className="py-2.5">{admin.created_by}</td>
                    {isSuperAdmin && (
                      <td className="py-2.5 text-right whitespace-nowrap">
                        {isSelf ? (
                          <span className="text-gray-300">—</span>
                        ) : (
                          <>
                            <button
                              type="button"
                              disabled={busy}
                              onClick={() =>
                                runAction(
                                  admin.id,
                                  admin.status === "active"
                                    ? () => pauseAdmin(admin.id)
                                    : () => resumeAdmin(admin.id),
                                )
                              }
                              className="text-gray-600 hover:text-gray-900 underline disabled:opacity-40 mr-3"
                            >
                              {admin.status === "active" ? "Pause" : "Resume"}
                            </button>
                            <button
                              type="button"
                              disabled={busy}
                              onClick={() => setConfirmDelete(admin)}
                              className="text-red-600 hover:text-red-800 underline disabled:opacity-40"
                            >
                              Delete
                            </button>
                          </>
                        )}
                      </td>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}

        <Pagination page={page} totalPages={totalPages} onChange={setPage} />
      </div>

      {modalOpen && (
        <Modal title="Add admin" onClose={() => setModalOpen(false)}>
          <CreateAdminForm onCreated={handleCreated} />
        </Modal>
      )}

      {confirmDelete && (
        <Modal title="Delete admin" onClose={() => setConfirmDelete(null)}>
          <p className="text-xs text-gray-600 mb-1">
            Permanently delete <span className="font-medium">{confirmDelete.email}</span>?
          </p>
          <p className="text-xs text-gray-400 mb-4">
            Their sessions end immediately and they lose dashboard access. Their audit history is kept. This cannot be
            undone — pause them instead if you may want them back.
          </p>
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={() => setConfirmDelete(null)}
              className="rounded-md border border-gray-300 px-4 py-1.5 text-xs font-medium hover:bg-gray-50"
            >
              Cancel
            </button>
            <button
              type="button"
              disabled={busyId === confirmDelete.id}
              onClick={() => runAction(confirmDelete.id, () => deleteAdmin(confirmDelete.id))}
              className="bg-red-600 text-white rounded-md px-4 py-1.5 text-xs font-medium disabled:opacity-40"
            >
              Delete admin
            </button>
          </div>
        </Modal>
      )}
    </>
  );
}

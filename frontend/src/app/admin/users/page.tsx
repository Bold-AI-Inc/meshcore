"use client";

import { useCallback, useEffect, useState } from "react";
import {
  deleteUser,
  listUsers,
  revokeUser,
  rotateUserKey,
  updateUserExpiry,
  type CreateUserResult,
  type UserListItem,
} from "@/lib/api";
import CopyableKeyReveal from "@/components/CopyableKeyReveal";
import CreateUserForm from "@/components/CreateUserForm";
import LogsDashboard from "@/components/LogsDashboard";
import Modal from "@/components/Modal";
import ModelAccessPanel from "@/components/ModelAccessPanel";
import Pagination from "@/components/Pagination";
import SearchInput from "@/components/SearchInput";
import StatusBadge from "@/components/StatusBadge";
import { usePaginatedFilter } from "@/hooks/usePaginatedFilter";

function toDateInputValue(iso: string | null): string {
  if (!iso) return "";
  return iso.slice(0, 10);
}

export default function UsersPage() {
  const [users, setUsers] = useState<UserListItem[]>([]);
  const [loadingUsers, setLoadingUsers] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [revealed, setRevealed] = useState<{ email: string; key: string } | null>(null);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [expiryDrafts, setExpiryDrafts] = useState<Record<string, string>>({});
  const [logsUser, setLogsUser] = useState<UserListItem | null>(null);

  function refreshUsers() {
    setLoadingUsers(true);
    listUsers()
      .then(setUsers)
      .catch(() => setUsers([]))
      .finally(() => setLoadingUsers(false));
  }

  useEffect(() => {
    refreshUsers();
  }, []);

  const filterFn = useCallback(
    (user: UserListItem, query: string) =>
      user.display_name.toLowerCase().includes(query) || user.email.toLowerCase().includes(query),
    [],
  );

  const { search, setSearch, page, setPage, totalPages, pageItems, totalCount } = usePaginatedFilter(
    users,
    filterFn,
  );

  function handleCreated(result: CreateUserResult) {
    setRevealed(result);
    setModalOpen(false);
    refreshUsers();
  }

  function toggleExpand(user: UserListItem) {
    setExpandedId((prev) => (prev === user.id ? null : user.id));
    setExpiryDrafts((prev) => ({ ...prev, [user.id]: toDateInputValue(user.expires_at) }));
  }

  async function handleSaveExpiry(userId: string) {
    const value = expiryDrafts[userId] ?? "";
    const iso = value ? new Date(`${value}T00:00:00Z`).toISOString() : null;
    await updateUserExpiry(userId, iso);
    refreshUsers();
  }

  async function handleRevoke(userId: string) {
    if (!confirm("Revoke this user's mesh key? Their key will stop working immediately.")) return;
    await revokeUser(userId);
    refreshUsers();
  }

  async function handleRotateKey(user: UserListItem) {
    const reactivateNote =
      user.status !== "active" ? " This will also reactivate their account (currently revoked)." : "";
    if (
      !confirm(
        `Rotate ${user.email}'s key? Their current key will stop working immediately and a new one will be generated.${reactivateNote}`,
      )
    )
      return;
    const result = await rotateUserKey(user.id);
    setRevealed({ email: user.email, key: result.key });
    refreshUsers();
  }

  async function handleDelete(userId: string) {
    if (!confirm("Delete this user permanently? This cannot be undone.")) return;
    await deleteUser(userId);
    setExpandedId(null);
    refreshUsers();
  }

  return (
    <>
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-lg font-serif">Users</h1>
        <button
          type="button"
          onClick={() => setModalOpen(true)}
          className="bg-black text-white rounded-md px-4 py-1.5 text-xs font-medium"
        >
          + Add user
        </button>
      </div>

      {revealed && (
        <CopyableKeyReveal
          email={revealed.email}
          keyPlaintext={revealed.key}
          onDismiss={() => setRevealed(null)}
        />
      )}

      <div className="bg-white rounded-lg shadow-sm p-4">
        <div className="flex items-center justify-between mb-3">
          <p className="text-xs text-gray-400">
            {totalCount} user{totalCount === 1 ? "" : "s"}
          </p>
          <SearchInput value={search} onChange={setSearch} placeholder="Search users..." />
        </div>

        {loadingUsers ? (
          <p className="text-xs text-gray-400">Loading...</p>
        ) : pageItems.length === 0 ? (
          <p className="text-xs text-gray-400">No users found</p>
        ) : (
          <div className="text-xs">
            <div className="grid grid-cols-[24px_1.2fr_1.6fr_1fr_1fr_1fr] gap-2 text-left text-gray-400 border-b border-gray-100 py-2 font-medium">
              <span></span>
              <span>Name</span>
              <span>Email</span>
              <span>Key</span>
              <span>Status</span>
              <span>Expires</span>
            </div>
            {pageItems.map((user) => {
              const isExpanded = expandedId === user.id;
              return (
                <div key={user.id} className="border-b border-gray-50 last:border-0">
                  <div
                    className="grid grid-cols-[24px_1.2fr_1.6fr_1fr_1fr_1fr] gap-2 items-center py-2.5 hover:bg-gray-50 cursor-pointer"
                    onClick={() => toggleExpand(user)}
                  >
                    <button
                      type="button"
                      aria-label={isExpanded ? "Collapse" : "Expand"}
                      className={`text-gray-400 transition-transform ${isExpanded ? "rotate-90" : ""}`}
                    >
                      &gt;
                    </button>
                    <span>{user.display_name}</span>
                    <span>{user.email}</span>
                    <span className="font-mono">{user.key_prefix ?? "—"}</span>
                    <span>
                      <StatusBadge status={user.status} />
                    </span>
                    <span>{user.expires_at ? toDateInputValue(user.expires_at) : "never"}</span>
                  </div>

                  {isExpanded && (
                    <div className="pl-8 pb-4 pr-2 space-y-4" onClick={(e) => e.stopPropagation()}>
                      <div className="border-t border-gray-100 pt-3">
                        <div className="flex items-center justify-between mb-2">
                          <p className="text-[11px] font-medium uppercase tracking-wide text-gray-400">
                            Expiry &amp; lifecycle
                          </p>
                          <div className="flex items-center gap-3">
                            <button
                              type="button"
                              onClick={() => setLogsUser(user)}
                              className="text-[11px] font-medium text-gray-600 hover:underline"
                            >
                              Logs
                            </button>
                            <button
                              type="button"
                              onClick={() => handleRotateKey(user)}
                              className="text-[11px] font-medium text-gray-600 hover:underline"
                            >
                              Rotate key
                            </button>
                            <button
                              type="button"
                              onClick={() => handleRevoke(user.id)}
                              className="text-[11px] font-medium text-amber-700 hover:underline"
                            >
                              Revoke key
                            </button>
                            <button
                              type="button"
                              onClick={() => handleDelete(user.id)}
                              className="text-[11px] font-medium text-red-600 hover:underline"
                            >
                              Delete user
                            </button>
                          </div>
                        </div>
                        <div className="flex items-center gap-2 flex-wrap">
                          <input
                            type="date"
                            value={expiryDrafts[user.id] ?? ""}
                            onChange={(e) =>
                              setExpiryDrafts((prev) => ({ ...prev, [user.id]: e.target.value }))
                            }
                            className="rounded-md border border-gray-300 px-2.5 py-1 text-xs outline-none focus:border-gray-500"
                          />
                          <button
                            type="button"
                            onClick={() => handleSaveExpiry(user.id)}
                            className="rounded-md border border-gray-300 px-2.5 py-1 text-xs font-medium hover:bg-gray-50"
                          >
                            Save
                          </button>
                          <button
                            type="button"
                            onClick={() => {
                              setExpiryDrafts((prev) => ({ ...prev, [user.id]: "" }));
                              updateUserExpiry(user.id, null).then(refreshUsers);
                            }}
                            className="rounded-md border border-gray-300 px-2.5 py-1 text-xs font-medium text-gray-500 hover:bg-gray-50"
                          >
                            Never expires
                          </button>
                        </div>
                      </div>

                      <div className="border-t border-gray-100 pt-3">
                        <p className="text-[11px] font-medium uppercase tracking-wide text-gray-400 mb-2">
                          Model access &amp; budget
                        </p>
                        <ModelAccessPanel userId={user.id} />
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}

        <Pagination page={page} totalPages={totalPages} onChange={setPage} />
      </div>

      {modalOpen && (
        <Modal title="Add user" onClose={() => setModalOpen(false)}>
          <CreateUserForm onCreated={handleCreated} />
        </Modal>
      )}

      {logsUser && (
        <LogsDashboard
          userId={logsUser.id}
          userLabel={`${logsUser.display_name} · ${logsUser.email}`}
          onClose={() => setLogsUser(null)}
        />
      )}
    </>
  );
}

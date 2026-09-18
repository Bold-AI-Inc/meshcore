"use client";

import { createContext, useContext } from "react";

export interface AdminSession {
  email: string;
  isSuperAdmin: boolean;
}

const AdminSessionContext = createContext<AdminSession>({ email: "", isSuperAdmin: false });

export const AdminSessionProvider = AdminSessionContext.Provider;

export function useAdminSession() {
  return useContext(AdminSessionContext);
}

export function useAdminEmail() {
  return useContext(AdminSessionContext).email;
}

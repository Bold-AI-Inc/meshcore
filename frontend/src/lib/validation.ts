import { z } from "zod";

export const changePasswordSchema = z
  .object({
    oldPassword: z.string().min(1, "Current password is required"),
    newPassword: z.string().min(8, "New password must be at least 8 characters"),
    confirmPassword: z.string().min(1, "Please confirm your new password"),
  })
  .refine((data) => data.newPassword === data.confirmPassword, {
    message: "Passwords do not match",
    path: ["confirmPassword"],
  })
  .refine((data) => data.oldPassword !== data.newPassword, {
    message: "New password must be different from the current password",
    path: ["newPassword"],
  });

export type ChangePasswordInput = z.infer<typeof changePasswordSchema>;

export const createUserSchema = z.object({
  email: z.string().min(1, "Email is required").email("Enter a valid email"),
  displayName: z.string().min(1, "Name is required"),
});

export type CreateUserInput = z.infer<typeof createUserSchema>;

export const createAdminSchema = z.object({
  email: z.string().min(1, "Email is required").email("Enter a valid email"),
  password: z.string().min(8, "Password must be at least 8 characters"),
});

export type CreateAdminInput = z.infer<typeof createAdminSchema>;

function isValidJson(value: string) {
  try {
    JSON.parse(value);
    return true;
  } catch {
    return false;
  }
}

export const modelDraftSchema = z.object({
  name: z.string().min(1, "Name is required"),
  resolvedModel: z.string().min(1, "Resolved model id is required"),
  inputPrice: z.coerce.number().min(0, "Cannot be negative"),
  outputPrice: z.coerce.number().min(0, "Cannot be negative"),
});

export const createProviderSchema = z.object({
  name: z.string().min(1, "Name is required"),
  apiKey: z.string().min(1, "API key is required"),
  endpointUrl: z.string().min(1, "Endpoint URL is required").url("Enter a valid URL"),
  httpMethod: z.string().min(1, "Method is required"),
  headerTemplate: z
    .string()
    .min(1, "Header template is required")
    .refine(isValidJson, "Must be valid JSON"),
  requestBodyTemplate: z
    .string()
    .min(1, "Request body template is required")
    .refine(isValidJson, "Must be valid JSON"),
  responseDeltaPath: z.string().min(1, "Response delta path is required"),
  usageInputTokensPath: z.string(),
  usageOutputTokensPath: z.string(),
  maxOutboundRps: z.coerce.number().int().min(1, "Must be at least 1"),
  maxConcurrentUpstream: z.coerce.number().int().min(1, "Must be at least 1"),
  maxRetries: z.coerce.number().int().min(0, "Cannot be negative"),
  retryBackoffMs: z.coerce.number().int().min(0, "Cannot be negative"),
  models: z.array(modelDraftSchema).min(1, "Add at least one model"),
});

export type CreateProviderInput = z.infer<typeof createProviderSchema>;

export const createModelSchema = z.object({
  providerId: z.string().min(1, "Select a provider"),
  ...modelDraftSchema.shape,
});

export type CreateModelInput = z.infer<typeof createModelSchema>;

export const updateProviderSchema = z.object({
  name: z.string().min(1, "Name is required"),
  apiKey: z.string(),
  endpointUrl: z.string().min(1, "Endpoint URL is required").url("Enter a valid URL"),
  httpMethod: z.string().min(1, "Method is required"),
  headerTemplate: z
    .string()
    .min(1, "Header template is required")
    .refine(isValidJson, "Must be valid JSON"),
  requestBodyTemplate: z
    .string()
    .min(1, "Request body template is required")
    .refine(isValidJson, "Must be valid JSON"),
  responseDeltaPath: z.string().min(1, "Response delta path is required"),
  usageInputTokensPath: z.string(),
  usageOutputTokensPath: z.string(),
  maxOutboundRps: z.coerce.number().int().min(1, "Must be at least 1"),
  maxConcurrentUpstream: z.coerce.number().int().min(1, "Must be at least 1"),
  maxRetries: z.coerce.number().int().min(0, "Cannot be negative"),
  retryBackoffMs: z.coerce.number().int().min(0, "Cannot be negative"),
});

export type UpdateProviderInput = z.infer<typeof updateProviderSchema>;

export const policySchema = z.object({
  dailyCapUsd: z.coerce.number().min(0, "Cannot be negative"),
  allowedFrom: z.string().optional(),
  allowedTo: z.string().optional(),
  timezone: z.string().min(1, "Timezone is required"),
  activeDays: z.array(z.number().int().min(0).max(6)),
  alwaysOpen: z.boolean(),
});

export type PolicyInput = z.infer<typeof policySchema>;

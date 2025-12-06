import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { Check, ChevronsUpDown } from "lucide-react";
import { useMemo, useState } from "react";
import type { UseFormReturn } from "react-hook-form";
import {
  CHANNEL_TYPES,
  CHANNEL_TYPES_WITH_CUSTOM_KEY_FIELD,
} from "../constants";
import { getKeyPrompt } from "../helpers";
import type { ChannelForm } from "../schemas";
import { resolveChannelColor } from "../utils/colorGenerator";
import { LabelWithHelp } from "./LabelWithHelp";

interface ChannelBasicInfoProps {
  form: UseFormReturn<ChannelForm>;
  groups: string[];
  normalizedChannelType: number | null;
  tr: (
    key: string,
    defaultValue: string,
    options?: Record<string, unknown>
  ) => string;
  /** Callback to request a type change (may trigger confirmation dialog in edit mode) */
  onTypeChange?: (newType: number) => void;
}

export const ChannelBasicInfo = ({
  form,
  groups,
  normalizedChannelType,
  tr,
  onTypeChange,
}: ChannelBasicInfoProps) => {
  const [typePopoverOpen, setTypePopoverOpen] = useState(false);
  const watchType = form.watch("type");
  const channelTypeOverridesKeyField =
    normalizedChannelType !== null &&
    CHANNEL_TYPES_WITH_CUSTOM_KEY_FIELD.has(normalizedChannelType);

  // Sort channel types alphabetically by text
  const sortedChannelTypes = useMemo(
    () => [...CHANNEL_TYPES].sort((a, b) => a.text.localeCompare(b.text)),
    []
  );

  const fieldHasError = (name: string) =>
    !!(form.formState.errors as any)?.[name];
  const errorClass = (name: string) =>
    fieldHasError(name)
      ? "border-destructive focus-visible:ring-destructive"
      : "";

  const toggleGroup = (groupValue: string) => {
    const currentGroups = form.getValues("groups");
    if (currentGroups.includes(groupValue)) {
      form.setValue(
        "groups",
        currentGroups.filter((g) => g !== groupValue)
      );
    } else {
      form.setValue("groups", [...currentGroups, groupValue]);
    }
  };

  const addGroup = (groupName: string) => {
    const currentGroups = form.getValues("groups");
    if (!currentGroups.includes(groupName)) {
      form.setValue("groups", [...currentGroups, groupName]);
    }
  };

  const removeGroup = (groupToRemove: string) => {
    const currentGroups = form.getValues("groups");
    const newGroups = currentGroups.filter((g) => g !== groupToRemove);
    // Ensure at least 'default' group remains
    if (newGroups.length === 0) {
      newGroups.push("default");
    }
    form.setValue("groups", newGroups);
  };

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
      <FormField
        control={form.control}
        name="name"
        render={({ field }) => (
          <FormItem>
            <LabelWithHelp
              label={tr("name.label", "Channel Name *")}
              help={tr(
                "name.help",
                "A descriptive name for this channel to identify it in logs and lists."
              )}
            />
            <FormControl>
              <Input
                placeholder={tr("name.placeholder", "My Channel")}
                className={errorClass("name")}
                {...field}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name="type"
        render={({ field }) => {
          const selectedType = sortedChannelTypes.find(
            (t) => t.value === field.value
          );
          const selectedColorValue = selectedType
            ? resolveChannelColor(selectedType.color, selectedType.value)
            : undefined;

          return (
            <FormItem>
              <LabelWithHelp
                label={tr("type.label", "Channel Type *")}
                help={tr(
                  "type.help",
                  "The provider type for this channel. Changing this may reset some fields."
                )}
              />
              <Popover open={typePopoverOpen} onOpenChange={setTypePopoverOpen}>
                <PopoverTrigger asChild>
                  <Button
                    type="button"
                    variant="outline"
                    role="combobox"
                    aria-expanded={typePopoverOpen}
                    className={cn(
                      "w-full justify-between font-normal",
                      !field.value && "text-muted-foreground",
                      errorClass("type")
                    )}
                  >
                    {selectedType ? (
                      <span className="flex items-center gap-2">
                        <span
                          className="inline-block w-2 h-2 rounded-full flex-shrink-0"
                          style={{ backgroundColor: selectedColorValue }}
                        />
                        {selectedType.text}
                      </span>
                    ) : (
                      tr("type.placeholder", "Select a channel type")
                    )}
                    <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent
                  className="p-0 min-w-[12rem] max-w-[calc(100vw-2rem)] overflow-hidden"
                  align="start"
                >
                  <Command
                    filter={(value, search) => {
                      // Use case-insensitive substring matching instead of fuzzy matching
                      if (value.toLowerCase().includes(search.toLowerCase()))
                        return 1;
                      return 0;
                    }}
                  >
                    <div className="[&_[cmdk-input]]:ring-0 [&_[cmdk-input]]:outline-none [&_[cmdk-input-wrapper]]:border-b-0">
                      <CommandInput
                        placeholder={tr(
                          "type.search",
                          "Search channel type..."
                        )}
                      />
                    </div>
                    <CommandList>
                      <CommandEmpty>
                        {tr("type.not_found", "No channel type found.")}
                      </CommandEmpty>
                      <CommandGroup>
                        {sortedChannelTypes.map((type) => {
                          const colorValue = resolveChannelColor(
                            type.color,
                            type.value
                          );
                          return (
                            <CommandItem
                              key={type.key}
                              value={type.text}
                              onSelect={() => {
                                if (onTypeChange) {
                                  onTypeChange(type.value);
                                } else {
                                  field.onChange(type.value);
                                }
                                setTypePopoverOpen(false);
                              }}
                            >
                              <Check
                                className={cn(
                                  "mr-2 h-4 w-4",
                                  field.value === type.value
                                    ? "opacity-100"
                                    : "opacity-0"
                                )}
                              />
                              <span className="flex items-center gap-2">
                                <span
                                  className="inline-block w-2 h-2 rounded-full flex-shrink-0"
                                  style={{ backgroundColor: colorValue }}
                                />
                                {type.text}
                              </span>
                            </CommandItem>
                          );
                        })}
                      </CommandGroup>
                    </CommandList>
                  </Command>
                </PopoverContent>
              </Popover>
              <FormMessage />
            </FormItem>
          );
        }}
      />

      <FormField
        control={form.control}
        name="groups"
        render={() => (
          <FormItem className="col-span-1 md:col-span-2">
            <LabelWithHelp
              label={tr("groups.label", "Groups *")}
              help={tr(
                "groups.help",
                'User groups that can access this channel. "default" is standard for normal users.'
              )}
            />
            <div className="flex flex-wrap gap-2 mb-2">
              {groups.map((group) => {
                const isSelected = form.watch("groups").includes(group);
                return (
                  <Badge
                    key={group}
                    variant={isSelected ? "default" : "outline"}
                    className="cursor-pointer hover:bg-primary/90"
                    onClick={() => toggleGroup(group)}
                  >
                    {group}
                  </Badge>
                );
              })}
            </div>
            <div className="flex gap-2">
              <Input
                placeholder={tr(
                  "groups.add_placeholder",
                  "Add custom group..."
                )}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    const val = (e.target as HTMLInputElement).value.trim();
                    if (val) {
                      addGroup(val);
                      (e.target as HTMLInputElement).value = "";
                    }
                  }
                }}
              />
            </div>
            <div className="flex flex-wrap gap-2 mt-2">
              {form.watch("groups").map((group) => (
                <Badge key={group} variant="secondary" className="gap-1">
                  {group}
                  <span
                    className="cursor-pointer ml-1 hover:text-destructive"
                    onClick={() => removeGroup(group)}
                  >
                    ×
                  </span>
                </Badge>
              ))}
            </div>
            <FormMessage />
          </FormItem>
        )}
      />

      {!channelTypeOverridesKeyField && (
        <FormField
          control={form.control}
          name="key"
          render={({ field }) => (
            <FormItem className="col-span-1 md:col-span-2">
              <LabelWithHelp
                label={tr("key.label", "API Key")}
                help={tr(
                  "key.help",
                  "The API key for authentication with the provider."
                )}
              />
              <FormControl>
                <Textarea
                  placeholder={getKeyPrompt(watchType)}
                  className={`font-mono text-sm ${errorClass("key")}`}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
    </div>
  );
};

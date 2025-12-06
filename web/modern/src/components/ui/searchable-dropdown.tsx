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
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Check, ChevronsUpDown, Loader2, Plus, X } from "lucide-react";
import * as React from "react";

export interface SearchOption {
  key: string;
  value: string;
  text: string;
  content?: React.ReactNode;
}

interface SearchableDropdownProps {
  value?: string;
  placeholder?: string;
  searchPlaceholder?: string;
  options: SearchOption[];
  onSearchChange?: (query: string) => void;
  onChange?: (value: string) => void;
  onAddItem?: (value: string) => void;
  loading?: boolean;
  noResultsMessage?: string;
  additionLabel?: string;
  allowAdditions?: boolean;
  clearable?: boolean;
  className?: string;
  // API-based search props
  searchEndpoint?: string;
  transformResponse?: (data: any[]) => SearchOption[];
  debounceMs?: number;
  minQueryLength?: number;
}

export function SearchableDropdown({
  value,
  placeholder = "Select option...",
  searchPlaceholder = "Search...",
  options: initialOptions,
  onSearchChange,
  onChange,
  onAddItem,
  loading = false,
  noResultsMessage = "No results found",
  additionLabel = "Add: ",
  allowAdditions = false,
  clearable = false,
  className,
  searchEndpoint,
  transformResponse,
  debounceMs = 300,
  minQueryLength = 2,
}: SearchableDropdownProps) {
  const [open, setOpen] = React.useState(false);
  const [searchValue, setSearchValue] = React.useState("");
  const [apiOptions, setApiOptions] = React.useState<SearchOption[]>([]);
  const [apiLoading, setApiLoading] = React.useState(false);
  const searchTimeoutRef = React.useRef<NodeJS.Timeout>();

  // Use API options if available, otherwise use initial options
  const options =
    searchEndpoint && searchValue.length >= minQueryLength
      ? apiOptions
      : initialOptions;

  const selectedOption = [...initialOptions, ...apiOptions].find(
    (option) => option.value === value
  );

  // API search with debouncing - now uses unified api wrapper
  const performApiSearch = React.useCallback(
    async (query: string) => {
      if (
        !searchEndpoint ||
        !transformResponse ||
        query.length < minQueryLength
      ) {
        setApiOptions([]);
        return;
      }

      setApiLoading(true);
      try {
        // Use unified api wrapper - caller provides complete URL including /api prefix
        const response = await api.get(
          `${searchEndpoint}?keyword=${encodeURIComponent(query)}`,
          {
            // Redundant safeguard: interceptor already adds these, but explicit for clarity
            headers: {
              "Cache-Control": "no-cache, no-store, must-revalidate",
              Pragma: "no-cache",
              Expires: "0",
            },
          }
        );
        const result = response.data;

        if (result.success && result.data) {
          const transformedOptions = transformResponse(result.data);
          setApiOptions(transformedOptions);
        } else {
          setApiOptions([]);
        }
      } catch (error) {
        console.error("API search failed:", error);
        setApiOptions([]);
      } finally {
        setApiLoading(false);
      }
    },
    [searchEndpoint, transformResponse, minQueryLength]
  );

  const handleSearchChange = (query: string) => {
    setSearchValue(query);
    onSearchChange?.(query);

    // Clear previous timeout
    if (searchTimeoutRef.current) {
      clearTimeout(searchTimeoutRef.current);
    }

    // Debounce API search
    if (searchEndpoint && transformResponse) {
      searchTimeoutRef.current = setTimeout(() => {
        performApiSearch(query);
      }, debounceMs);
    }
  };

  const handleSelect = (selectedValue: string) => {
    if (selectedValue === value && clearable) {
      onChange?.("");
    } else {
      onChange?.(selectedValue);
    }
    setOpen(false);
    setSearchValue(""); // Clear search when selecting
  };

  const handleAddItem = () => {
    if (searchValue && allowAdditions && onAddItem) {
      onAddItem(searchValue);
      onChange?.(searchValue);
      setSearchValue("");
      setOpen(false);
    }
  };

  const handleClear = (e: React.MouseEvent) => {
    e.stopPropagation();
    onChange?.("");
  };

  // Local filtering for initial options when not using API search
  const filteredOptions = React.useMemo(() => {
    if (searchEndpoint || !searchValue) return options;
    return options.filter(
      (option) =>
        option.text.toLowerCase().includes(searchValue.toLowerCase()) ||
        option.value.toLowerCase().includes(searchValue.toLowerCase())
    );
  }, [options, searchValue, searchEndpoint]);

  const showAddition =
    allowAdditions &&
    searchValue &&
    !filteredOptions.some(
      (option) => option.value.toLowerCase() === searchValue.toLowerCase()
    );

  const currentLoading = loading || apiLoading;

  // Cleanup timeout on unmount
  React.useEffect(() => {
    return () => {
      if (searchTimeoutRef.current) {
        clearTimeout(searchTimeoutRef.current);
      }
    };
  }, []);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className={cn("w-full justify-between", className)}
        >
          {selectedOption ? (
            <span className="truncate">{selectedOption.text}</span>
          ) : (
            <span className="text-muted-foreground">{placeholder}</span>
          )}
          <div className="flex items-center ml-2 shrink-0">
            {clearable && selectedOption && (
              <X
                className="h-4 w-4 opacity-50 hover:opacity-100 mr-1"
                onClick={handleClear}
              />
            )}
            <ChevronsUpDown className="h-4 w-4 opacity-50" />
          </div>
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className={cn(
          // Avoid horizontal overflow on small screens: don't use w-full (which equals 100vw in portal)
          // Instead, cap width to viewport with padding and allow it to size naturally on larger screens
          "p-0 min-w-[12rem] max-w-[calc(100vw-2rem)] sm:w-auto"
        )}
        align="start"
      >
        <Command shouldFilter={!searchEndpoint}>
          <CommandInput
            placeholder={searchPlaceholder}
            value={searchValue}
            onValueChange={handleSearchChange}
          />
          <CommandList>
            {currentLoading ? (
              <div className="flex items-center justify-center py-6">
                <Loader2 className="h-4 w-4 animate-spin" />
                <span className="ml-2 text-sm text-muted-foreground">
                  Searching...
                </span>
              </div>
            ) : (
              <>
                {filteredOptions.length === 0 && !showAddition ? (
                  <CommandEmpty>
                    {searchValue.length < minQueryLength && searchEndpoint
                      ? `Type at least ${minQueryLength} characters to search`
                      : noResultsMessage}
                  </CommandEmpty>
                ) : (
                  <CommandGroup>
                    {filteredOptions.map((option) => (
                      <CommandItem
                        key={option.key}
                        value={option.value}
                        onSelect={handleSelect}
                        className="cursor-pointer"
                      >
                        <Check
                          className={cn(
                            "mr-2 h-4 w-4",
                            value === option.value ? "opacity-100" : "opacity-0"
                          )}
                        />
                        <div className="flex-1">
                          {option.content || option.text}
                        </div>
                      </CommandItem>
                    ))}
                    {showAddition && (
                      <CommandItem
                        onSelect={handleAddItem}
                        className="cursor-pointer"
                      >
                        <Plus className="mr-2 h-4 w-4" />
                        <span className="text-muted-foreground">
                          {additionLabel}
                        </span>
                        <span className="font-medium">{searchValue}</span>
                      </CommandItem>
                    )}
                  </CommandGroup>
                )}
              </>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

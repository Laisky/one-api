import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useNotifications } from '@/components/ui/notifications';
import { generateUUIDv4 } from '@/lib/utils';
import { AlertCircle, Eye, FileX, ImageIcon, Upload, X } from 'lucide-react';
import React, { useCallback, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface ImageAttachment {
  id: string;
  file: File;
  preview: string;
  base64: string;
  thumbnail?: string;
  compressed?: boolean;
}

interface ValidationError {
  fileName: string;
  reason: string;
  type: 'file_type' | 'file_size' | 'file_limit' | 'processing_error';
}

interface ImageAttachmentProps {
  images: ImageAttachment[];
  onImagesChange: (images: ImageAttachment[]) => void;
  disabled?: boolean;
  maxImages?: number;
}

export function ImageAttachmentComponent({ images, onImagesChange, disabled = false, maxImages = 5 }: ImageAttachmentProps) {
  const { t } = useTranslation();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [isProcessing, setIsProcessing] = useState(false);
  const [processingProgress, setProcessingProgress] = useState(0);
  const [validationErrors, setValidationErrors] = useState<ValidationError[]>([]);
  const abortControllerRef = useRef<AbortController | null>(null);
  const progressResetTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const { notify } = useNotifications();

  // Comprehensive file validation with detailed error reporting
  const validateFile = useCallback(
    (file: File, currentImageCount: number): ValidationError | null => {
      // Check file type via MIME type
      if (!file.type.startsWith('image/')) {
        return {
          fileName: file.name,
          reason: t('playground.attachments.validation.file_type_unsupported', {
            type: file.type || t('playground.attachments.validation.unknown_type'),
          }),
          type: 'file_type',
        };
      }

      // Additional file extension validation as backup
      const validExtensions = ['.jpg', '.jpeg', '.png', '.gif', '.webp', '.bmp', '.svg'];
      const fileName = file.name.toLowerCase();
      const hasValidExtension = validExtensions.some((ext) => fileName.endsWith(ext));

      if (!hasValidExtension) {
        return {
          fileName: file.name,
          reason: t('playground.attachments.validation.extension_unsupported', { formats: validExtensions.join(', ') }),
          type: 'file_type',
        };
      }

      // Check file size (5MB limit)
      const maxSize = 5 * 1024 * 1024;
      if (file.size > maxSize) {
        const fileSize = (file.size / (1024 * 1024)).toFixed(2);
        return {
          fileName: file.name,
          reason: t('playground.attachments.validation.size_exceeded', { size: fileSize }),
          type: 'file_size',
        };
      }

      // Check if adding this file would exceed the image limit
      if (currentImageCount >= maxImages) {
        return {
          fileName: file.name,
          reason: t('playground.attachments.validation.limit_exceeded', { max: maxImages }),
          type: 'file_limit',
        };
      }

      return null; // File is valid
    },
    [maxImages, t]
  );

  // Mobile device detection
  const isMobile = useMemo(() => {
    if (typeof window === 'undefined') return false;
    return (
      /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent) ||
      window.innerWidth <= 768 ||
      'ontouchstart' in window
    );
  }, []);

  // Optimized image compression with mobile-specific handling
  const compressImage = useCallback(
    async (file: File, maxWidth = 1920, maxHeight = 1080, quality = 0.8): Promise<{ blob: Blob; base64: string }> => {
      return new Promise((resolve, reject) => {
        // Mobile-specific timeout to prevent hanging
        const timeout = setTimeout(
          () => {
            reject(new Error(`Image compression timeout after ${isMobile ? '15' : '30'} seconds`));
          },
          isMobile ? 15000 : 30000
        );

        const cleanup = () => {
          clearTimeout(timeout);
          if (objectUrl) URL.revokeObjectURL(objectUrl);
        };

        const canvas = document.createElement('canvas');
        const ctx = canvas.getContext('2d', {
          alpha: false, // Optimize for JPEG images
          willReadFrequently: false, // Optimize for one-time operations
        });
        const img = new Image();
        let objectUrl: string;

        img.onload = () => {
          try {
            // Mobile-specific size limits to prevent memory issues
            const mobileMaxWidth = isMobile ? Math.min(maxWidth, 1200) : maxWidth;
            const mobileMaxHeight = isMobile ? Math.min(maxHeight, 1200) : maxHeight;

            // Calculate optimal dimensions
            let { width, height } = img;
            if (width > mobileMaxWidth || height > mobileMaxHeight) {
              const ratio = Math.min(mobileMaxWidth / width, mobileMaxHeight / height);
              width *= ratio;
              height *= ratio;
            }

            // Additional mobile memory check
            const pixelCount = width * height;
            const maxPixels = isMobile ? 1440000 : 2073600; // 1200x1200 for mobile, 1440x1440 for desktop
            if (pixelCount > maxPixels) {
              const scaleFactor = Math.sqrt(maxPixels / pixelCount);
              width *= scaleFactor;
              height *= scaleFactor;
            }

            canvas.width = width;
            canvas.height = height;

            // Draw and compress with error handling
            ctx?.drawImage(img, 0, 0, width, height);

            // Mobile-specific quality adjustment
            const mobileQuality = isMobile ? Math.min(quality, 0.7) : quality;

            canvas.toBlob(
              async (blob) => {
                if (blob) {
                  try {
                    // Use canvas toDataURL directly - simpler and more reliable
                    const base64 = canvas.toDataURL(file.type.startsWith('image/png') ? 'image/png' : 'image/jpeg', mobileQuality);

                    cleanup();
                    resolve({ blob, base64 });
                  } catch (conversionError) {
                    cleanup();
                    reject(
                      new Error(`Base64 conversion failed: ${conversionError instanceof Error ? conversionError.message : 'Unknown error'}`)
                    );
                  }
                } else {
                  cleanup();
                  reject(new Error('Canvas toBlob failed - browser may be out of memory'));
                }
              },
              file.type.startsWith('image/png') ? 'image/png' : 'image/jpeg',
              mobileQuality
            );
          } catch (error) {
            cleanup();
            reject(error);
          }
        };

        img.onerror = (error) => {
          cleanup();
          reject(new Error(`Image load error: ${error}`));
        };

        try {
          objectUrl = URL.createObjectURL(file);
          img.src = objectUrl;
        } catch (error) {
          cleanup();
          reject(new Error(`Failed to create object URL: ${error}`));
        }
      });
    },
    [isMobile]
  );

  // Generate optimized thumbnail
  const generateThumbnail = useCallback(
    async (file: File): Promise<string> => {
      const { base64 } = await compressImage(file, 150, 150, 0.6);
      return base64;
    },
    [compressImage]
  );

  // Simple canvas-only file to base64 conversion
  const fileToBase64 = useCallback(async (file: File): Promise<string> => {
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        cleanup();
        reject(new Error('File processing timeout'));
      }, 20000);

      let objectUrl: string | null = null;

      const cleanup = () => {
        clearTimeout(timeout);
        if (objectUrl) {
          URL.revokeObjectURL(objectUrl);
        }
      };

      try {
        objectUrl = URL.createObjectURL(file);
        const img = new Image();

        img.onload = () => {
          try {
            const canvas = document.createElement('canvas');
            const ctx = canvas.getContext('2d');

            if (!ctx) {
              cleanup();
              reject(new Error('Could not get canvas context'));
              return;
            }

            canvas.width = img.naturalWidth || img.width;
            canvas.height = img.naturalHeight || img.height;
            ctx.drawImage(img, 0, 0);

            const dataURL = canvas.toDataURL(file.type.startsWith('image/png') ? 'image/png' : 'image/jpeg', 0.9);
            cleanup();
            resolve(dataURL);
          } catch (canvasError) {
            cleanup();
            reject(new Error(`Canvas conversion failed: ${canvasError instanceof Error ? canvasError.message : 'Unknown error'}`));
          }
        };

        img.onerror = () => {
          cleanup();
          reject(new Error('Image load failed'));
        };

        img.src = objectUrl;
      } catch (error) {
        cleanup();
        reject(new Error(`File processing failed: ${error instanceof Error ? error.message : 'Unknown error'}`));
      }
    });
  }, []);

  // Mobile-aware image processing with fallback strategies
  const processImagesInParallel = useCallback(
    async (files: File[]): Promise<ImageAttachment[]> => {
      if (files.length === 0) return [];

      const results: ImageAttachment[] = [];
      let completed = 0;
      let processingErrors = 0;

      const processImage = async (file: File, index: number): Promise<ImageAttachment | null> => {
        const startTime = Date.now();

        try {
          // Validate file type and size early
          if (!file.type.startsWith('image/') || file.size > 5 * 1024 * 1024) {
            completed++;
            setProcessingProgress((completed / files.length) * 100);
            return null;
          }

          // Mobile-specific processing strategy to prevent memory issues
          let thumbnail: string;
          let compressed: { blob: Blob; base64: string };

          if (isMobile) {
            // Skip optimization on mobile - use original file directly
            try {
              compressed = { blob: file, base64: await fileToBase64(file) };
              // No thumbnail generation for mobile
              thumbnail = '';
            } catch (mobileError) {
              // Enhanced mobile error handling
              notify({
                type: 'error',
                title: t('playground.attachments.notifications.mobile_processing_failed_title'),
                message: t('playground.attachments.notifications.mobile_processing_failed_message', {
                  file: file.name,
                  message: mobileError instanceof Error ? mobileError.message : t('playground.attachments.validation.unknown_error'),
                }),
                durationMs: 6000,
              });
              throw mobileError;
            }
          } else {
            // Parallel processing on desktop
            try {
              const [thumbnailResult, compressedResult] = await Promise.race([
                Promise.all([generateThumbnail(file), compressImage(file)]),
                // Add timeout for parallel operations
                new Promise<never>((_, reject) => setTimeout(() => reject(new Error('Processing timeout')), 25000)),
              ]);
              thumbnail = thumbnailResult;
              compressed = compressedResult;
            } catch (error) {
              // Fallback to sequential processing
              compressed = await compressImage(file);
              thumbnail = compressed.base64;
            }
          }

          const preview = URL.createObjectURL(file);
          const processingTime = Date.now() - startTime;

          completed++;
          setProcessingProgress((completed / files.length) * 100);

          return {
            id: generateUUIDv4(),
            file,
            preview,
            base64: compressed.base64,
            thumbnail,
            compressed: compressed.blob !== file,
          };
        } catch (error) {
          processingErrors++;
          notify({
            type: 'error',
            title: t('playground.attachments.notifications.image_processing_failed_title'),
            message: t('playground.attachments.notifications.image_processing_failed_message', {
              file: file.name,
              message: error instanceof Error ? error.message : t('playground.attachments.validation.unknown_error'),
            }),
            durationMs: 5000,
          });

          // If too many errors, switch to simpler processing
          if (processingErrors > 1 && isMobile) {
            try {
              const simpleBase64 = await fileToBase64(file);
              const preview = URL.createObjectURL(file);

              completed++;
              setProcessingProgress((completed / files.length) * 100);

              return {
                id: generateUUIDv4(),
                file,
                preview,
                base64: simpleBase64,
                thumbnail: simpleBase64,
                compressed: false,
              };
            } catch (fallbackError) {
              notify({
                type: 'error',
                title: t('playground.attachments.notifications.all_processing_failed_title'),
                message: t('playground.attachments.notifications.all_processing_failed_message', { file: file.name }),
                durationMs: 7000,
              });
            }
          }

          completed++;
          setProcessingProgress((completed / files.length) * 100);
          return null;
        }
      };

      // Mobile-aware concurrency limits
      const concurrencyLimit = isMobile ? 1 : Math.min(3, navigator.hardwareConcurrency || 2);
      const batchDelay = isMobile ? 50 : 10; // Longer delays on mobile

      for (let i = 0; i < files.length; i += concurrencyLimit) {
        const batch = files.slice(i, i + concurrencyLimit);

        try {
          const batchResults = await Promise.allSettled(batch.map((file, batchIndex) => processImage(file, i + batchIndex)));

          batchResults.forEach((result) => {
            if (result.status === 'fulfilled' && result.value) {
              results.push(result.value);
            }
          });
        } catch (batchError) {
          console.error(`Batch processing error:`, batchError);
          // Continue with next batch instead of failing completely
        }

        // Check for cancellation
        if (abortControllerRef.current?.signal.aborted) {
          break;
        }

        // Progressive delay increase if errors are accumulating
        const delay = batchDelay + processingErrors * 100;
        if (i + concurrencyLimit < files.length) {
          await new Promise((resolve) => setTimeout(resolve, delay));
        }

        // Memory cleanup hint for mobile devices
        if (isMobile && (i + concurrencyLimit) % 2 === 0) {
          // Suggest garbage collection every 2 batches on mobile
          if (window.gc) {
            window.gc();
          }
        }
      }

      return results;
    },
    [generateThumbnail, compressImage, fileToBase64, isMobile, notify, t]
  );

  // Debounced file selection to prevent rapid consecutive calls
  const debouncedHandleFileSelect = useMemo(() => {
    let timeoutId: NodeJS.Timeout;
    return (event: React.ChangeEvent<HTMLInputElement>) => {
      clearTimeout(timeoutId);
      timeoutId = setTimeout(() => handleFileSelect(event), 100);
    };
  }, []);

  const handleFileSelect = useCallback(
    async (event: React.ChangeEvent<HTMLInputElement>) => {
      const files = Array.from(event.target.files || []);
      if (files.length === 0) return;

      // Cancel any ongoing processing
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
      abortControllerRef.current = new AbortController();

      // Clear any existing progress reset timeout
      if (progressResetTimeoutRef.current) {
        clearTimeout(progressResetTimeoutRef.current);
        progressResetTimeoutRef.current = null;
      }

      // Clear previous validation errors
      setValidationErrors([]);

      // Validate all files and collect errors
      const errors: ValidationError[] = [];
      const validFiles: File[] = [];
      let currentImageCount = images.length;

      files.forEach((file) => {
        const validationError = validateFile(file, currentImageCount);
        if (validationError) {
          errors.push(validationError);
        } else {
          // Only add valid files and increment counter if we haven't reached the limit
          if (validFiles.length < maxImages - images.length) {
            validFiles.push(file);
            currentImageCount++;
          } else {
            // Add limit error for additional valid files
            errors.push({
              fileName: file.name,
              reason: t('playground.attachments.validation.limit_skipped', { max: maxImages }),
              type: 'file_limit',
            });
          }
        }
      });

      // Show validation errors if any
      if (errors.length > 0) {
        setValidationErrors(errors);
      }

      // Process valid files if any
      if (validFiles.length === 0) {
        // Reset file input even if no files were processed
        if (fileInputRef.current) {
          fileInputRef.current.value = '';
        }
        return;
      }

      setIsProcessing(true);
      setProcessingProgress(0);

      try {
        const newImages = await processImagesInParallel(validFiles);

        if (!abortControllerRef.current.signal.aborted) {
          // Clear any existing progress reset timeout
          if (progressResetTimeoutRef.current) {
            clearTimeout(progressResetTimeoutRef.current);
          }

          if (newImages.length > 0) {
            onImagesChange([...images, ...newImages]);
            // Keep progress at 100% briefly to show completion
            progressResetTimeoutRef.current = setTimeout(() => {
              setProcessingProgress(0);
              progressResetTimeoutRef.current = null;
            }, 1000);
          } else {
            // If no images were processed successfully, show completion immediately
            setProcessingProgress(100);
            progressResetTimeoutRef.current = setTimeout(() => {
              setProcessingProgress(0);
              progressResetTimeoutRef.current = null;
            }, 2000);
          }
        }
      } catch (error) {
        console.error('Error processing images:', error);
        // Add processing error to validation errors
        setValidationErrors((prev) => [
          ...prev,
          {
            fileName: 'Unknown',
            reason: t('playground.attachments.validation.processing_error'),
            type: 'processing_error',
          },
        ]);
      } finally {
        setIsProcessing(false);
        // Don't reset progress immediately - let the setTimeout handle it
        abortControllerRef.current = null;

        // Reset file input
        if (fileInputRef.current) {
          fileInputRef.current.value = '';
        }
      }
    },
    [images, maxImages, processImagesInParallel, onImagesChange, validateFile, t]
  );

  // Cancel processing when component unmounts
  React.useEffect(() => {
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
      if (progressResetTimeoutRef.current) {
        clearTimeout(progressResetTimeoutRef.current);
      }
    };
  }, []);

  const handleRemoveImage = (imageId: string) => {
    const imageToRemove = images.find((img) => img.id === imageId);
    if (imageToRemove) {
      URL.revokeObjectURL(imageToRemove.preview);
    }
    onImagesChange(images.filter((img) => img.id !== imageId));
  };

  const handleButtonClick = () => {
    fileInputRef.current?.click();
  };

  const formatFileSize = (bytes: number): string => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  };

  return (
    <div className="space-y-2 sm:space-y-3">
      {/* Image attachment button */}
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={handleButtonClick}
          disabled={disabled || isProcessing || images.length >= maxImages}
          className="flex items-center gap-2 hover:bg-primary/10"
        >
          {isProcessing ? (
            <>
              <div className="animate-spin h-4 w-4 border-2 border-primary border-t-transparent rounded-full" />
              {isMobile
                ? processingProgress < 100
                  ? t('playground.attachments.optimizing', { progress: Math.round(processingProgress) })
                  : t('playground.attachments.finalizing')
                : t('playground.attachments.processing', { progress: Math.round(processingProgress) })}
            </>
          ) : (
            <>
              <ImageIcon className="h-4 w-4" />
              {isMobile ? t('playground.attachments.add_images') : t('playground.attachments.attach_images')}
            </>
          )}
        </Button>

        {images.length > 0 && (
          <Badge variant="secondary" className="text-xs">
            {t('playground.attachments.count', { count: images.length, max: maxImages })}
          </Badge>
        )}
      </div>

      {/* Processing progress bar */}
      {isProcessing && (
        <div className="w-full bg-muted rounded-full h-2">
          <div className="bg-primary h-2 rounded-full transition-all duration-300 ease-out" style={{ width: `${processingProgress}%` }} />
        </div>
      )}

      {/* Validation Errors Display */}
      {validationErrors.length > 0 && (
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-destructive">
            <AlertCircle className="h-4 w-4" />
            <span className="text-sm font-medium">
              {validationErrors.length === 1
                ? t('playground.attachments.validation.single_error')
                : t('playground.attachments.validation.multiple_errors', { count: validationErrors.length })}
            </span>
          </div>
          <div className="space-y-2 max-h-32 overflow-y-auto">
            {validationErrors.map((error, index) => (
              <Card key={index} className="border-destructive/20 bg-destructive/5">
                <CardContent className="p-3">
                  <div className="flex items-start gap-2">
                    <FileX className="h-4 w-4 text-destructive mt-0.5 flex-shrink-0" />
                    <div className="flex-1 min-w-0">
                      <div className="text-sm font-medium text-destructive truncate" title={error.fileName}>
                        {error.fileName}
                      </div>
                      <div className="text-xs text-destructive/80 mt-1">{error.reason}</div>
                    </div>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setValidationErrors((prev) => prev.filter((_, i) => i !== index))}
                      className="h-6 w-6 p-0 text-destructive hover:text-destructive/80 hover:bg-destructive/10"
                      title={t('playground.attachments.validation.dismiss_error')}
                      aria-label={t('playground.attachments.validation.dismiss_error')}
                    >
                      <X className="h-3 w-3" />
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
          <div className="text-xs text-muted-foreground">{t('playground.attachments.validation.tip')}</div>
        </div>
      )}

      {/* Hidden file input */}
      <input ref={fileInputRef} type="file" multiple accept="image/*" onChange={debouncedHandleFileSelect} className="hidden" />

      {/* Image previews */}
      {images.length > 0 && (
        <>
          {isMobile ? (
            /* Mobile: Text-based file list without image previews */
            <div className="space-y-2">
              {images.map((image) => (
                <Card key={image.id} className="relative">
                  <CardContent className="p-3">
                    <div className="flex items-center justify-between gap-3">
                      <div className="flex items-center gap-3 flex-1 min-w-0">
                        <ImageIcon className="h-5 w-5 text-muted-foreground flex-shrink-0" />
                        <div className="flex-1 min-w-0">
                          <div className="text-sm font-medium truncate" title={image.file.name}>
                            {image.file.name}
                          </div>
                          <div className="text-xs text-muted-foreground">{formatFileSize(image.file.size)}</div>
                        </div>
                      </div>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => handleRemoveImage(image.id)}
                        className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
                        title={t('playground.attachments.remove_image')}
                        aria-label={t('playground.attachments.remove_image')}
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          ) : (
            /* Desktop: Image grid with thumbnails */
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-2 sm:gap-3">
              {images.map((image) => (
                <Card key={image.id} className="relative group overflow-hidden">
                  <CardContent className="p-0">
                    <div className="relative aspect-square">
                      <img
                        src={image.thumbnail || image.preview}
                        alt={image.file.name}
                        className="w-full h-full object-cover rounded-lg"
                        loading="lazy"
                        onError={(e) => {
                          // Fallback to preview if thumbnail fails
                          const target = e.target as HTMLImageElement;
                          if (target.src !== image.preview) {
                            target.src = image.preview;
                          }
                        }}
                      />

                      {/* Overlay with file info */}
                      <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity duration-200 flex items-end">
                        <div className="p-2 w-full">
                          <div className="text-white text-xs font-medium truncate">{image.file.name}</div>
                          <div className="text-white/80 text-xs">{formatFileSize(image.file.size)}</div>
                        </div>
                      </div>

                      {/* Remove button */}
                      <Button
                        type="button"
                        variant="destructive"
                        size="sm"
                        onClick={() => handleRemoveImage(image.id)}
                        className="absolute top-1 right-1 h-6 w-6 p-0 opacity-0 group-hover:opacity-100 transition-opacity duration-200"
                        title={t('playground.attachments.remove_image')}
                        aria-label={t('playground.attachments.remove_image')}
                      >
                        <X className="h-3 w-3" />
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}
        </>
      )}

      {/* Info text */}
      <div className="text-xs text-muted-foreground">{t('playground.attachments.info', { max: maxImages })}</div>
    </div>
  );
}

export type { ImageAttachment };

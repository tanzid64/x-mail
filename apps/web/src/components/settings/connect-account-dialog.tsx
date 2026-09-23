import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Loader2, Plus } from 'lucide-react'

import { FieldError, FormError } from '@/components/auth/field-error'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useConnectAccount } from '@/features/accounts/api'
import { ACCOUNT_COLORS, accountDotClass } from '@/features/accounts/types'
import { applyApiErrors } from '@/lib/form'
import { toast } from '@/lib/toast'
import { cn } from '@/lib/utils'

const schema = z.object({
  name: z.string().min(1, 'Give this account a name.').max(80, 'Must be at most 80 characters.'),
  api_key: z.string().min(1, 'Paste the API key for this provider.'),
  color: z.string(),
})

type ConnectValues = z.infer<typeof schema>

export function ConnectAccountDialog() {
  const [open, setOpen] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const connect = useConnectAccount()

  const {
    register,
    handleSubmit,
    setError,
    reset,
    setValue,
    watch,
    formState: { errors, isSubmitting },
  } = useForm<ConnectValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', api_key: '', color: 'emerald' },
  })

  const color = watch('color')

  const onSubmit = handleSubmit(async (values) => {
    setFormError(null)
    try {
      // The key is verified against the provider before anything is stored, so
      // a failure here means the key itself is wrong.
      const account = await connect.mutateAsync({ ...values, provider: 'resend' })
      reset()
      setOpen(false)
      toast.success(`“${account.name}” is connected. Sync its domains to start sending.`)
    } catch (error) {
      setFormError(applyApiErrors(error, setError))
    }
  })

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) {
          reset()
          setFormError(null)
        }
      }}
    >
      <DialogTrigger asChild>
        <Button className="gap-2">
          <Plus className="size-4" />
          Add mail account
        </Button>
      </DialogTrigger>

      <DialogContent>
        <form onSubmit={onSubmit} noValidate>
          <DialogHeader>
            <DialogTitle>Add a mail account</DialogTitle>
            <DialogDescription>
              Connect a provider account. The key is checked before it is saved, and it is stored
              encrypted — it is never sent back to the browser.
            </DialogDescription>
          </DialogHeader>

          <div className="my-6 flex flex-col gap-4">
            <FormError message={formError} />

            <div className="flex flex-col gap-2">
              <Label htmlFor="account-name">Name</Label>
              <Input id="account-name" placeholder="Work" autoFocus {...register('name')} />
              <FieldError message={errors.name?.message} />
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="account-provider">Provider</Label>
              <Select defaultValue="resend" disabled>
                <SelectTrigger id="account-provider">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="resend">Resend</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">More providers are planned.</p>
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="account-key">API key</Label>
              <Input
                id="account-key"
                type="password"
                autoComplete="off"
                placeholder="re_…"
                {...register('api_key')}
              />
              <FieldError message={errors.api_key?.message} />
            </div>

            <div className="flex flex-col gap-2">
              <Label>Colour</Label>
              <div className="flex gap-2">
                {ACCOUNT_COLORS.map((option) => (
                  <button
                    key={option}
                    type="button"
                    aria-label={option}
                    aria-pressed={color === option}
                    onClick={() => setValue('color', option)}
                    className={cn(
                      'size-7 rounded-full ring-offset-2 transition',
                      accountDotClass(option),
                      color === option ? 'ring-2 ring-foreground' : 'opacity-60 hover:opacity-100',
                    )}
                  />
                ))}
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? <Loader2 className="size-4 animate-spin" /> : null}
              Connect
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

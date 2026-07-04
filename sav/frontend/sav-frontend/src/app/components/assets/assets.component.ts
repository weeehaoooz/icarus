import { Component, inject, OnInit, signal, computed, ViewChild } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { AgGridAngular } from 'ag-grid-angular';
import {
  ColDef,
  GridReadyEvent,
  CellValueChangedEvent,
  GridApi,
} from 'ag-grid-community';
import { FinanceService, Asset, AssetClass } from '../../services/finance.service';

@Component({
  selector: 'app-assets',
  imports: [CommonModule, ReactiveFormsModule, AgGridAngular],
  templateUrl: './assets.component.html',
  styleUrl: './assets.component.scss'
})
export class AssetsComponent implements OnInit {
  protected readonly finance = inject(FinanceService);
  private readonly fb = inject(FormBuilder);

  // Grid API
  private gridApi!: GridApi;

  // Signals
  protected readonly isAdding = signal(false);
  protected readonly uploadError = signal<string | null>(null);
  protected readonly importResult = signal<{ imported: number; skipped: number; errors?: any[] } | null>(null);

  // Form setup for adding assets
  protected readonly assetForm = this.fb.nonNullable.group({
    name: ['', [Validators.required]],
    ticker: [''],
    isin: [''],
    asset_class: ['cash' as AssetClass, [Validators.required]],
    quantity: ['0', [Validators.required, Validators.pattern(/^[0-9]+(\.[0-9]+)?$/)]],
    unit_cost: ['0', [Validators.required, Validators.pattern(/^[0-9]+(\.[0-9]+)?$/)]],
    currency: ['SGD', [Validators.required]],
    exchange: [''],
    country: [''],
    notes: ['']
  });

  // AG Grid column definitions
  protected readonly columnDefs: ColDef[] = [
    {
      field: 'name',
      headerName: 'Asset Name',
      editable: true,
      sortable: true,
      filter: true,
      minWidth: 150
    },
    {
      field: 'ticker',
      headerName: 'Ticker',
      editable: true,
      width: 100
    },
    {
      field: 'asset_class',
      headerName: 'Class',
      editable: true,
      width: 120,
      cellEditor: 'agSelectCellEditor',
      cellEditorParams: {
        values: ['cash', 'equity', 'bond', 'real_estate', 'crypto', 'commodity', 'alternative']
      }
    },
    {
      field: 'quantity',
      headerName: 'Quantity',
      editable: true,
      width: 110,
      valueParser: params => params.newValue,
      cellClass: 'numeric-cell'
    },
    {
      field: 'unit_cost',
      headerName: 'Unit Cost',
      editable: true,
      width: 110,
      valueParser: params => params.newValue,
      cellClass: 'numeric-cell'
    },
    {
      field: 'current_value',
      headerName: 'Current Value (SGD)',
      width: 160,
      valueGetter: params => {
        const qty = parseFloat(params.data.quantity) || 0;
        const price = params.data.live_price ? parseFloat(params.data.live_price) : (parseFloat(params.data.unit_cost) || 0);
        return (qty * price).toFixed(2);
      },
      cellClass: 'numeric-cell font-weight-bold'
    },
    {
      field: 'live_price',
      headerName: 'Live Price',
      width: 110,
      valueFormatter: params => params.value ? `SGD ${parseFloat(params.value).toFixed(2)}` : '-',
      cellClass: 'numeric-cell text-info'
    },
    {
      field: 'currency',
      headerName: 'Currency',
      editable: true,
      width: 100
    },
    {
      field: 'notes',
      headerName: 'Notes',
      editable: true,
      flex: 1,
      minWidth: 150
    },
    {
      headerName: 'Actions',
      width: 90,
      cellRenderer: (params: any) => {
        const btn = document.createElement('button');
        btn.innerHTML = 'Delete';
        btn.className = 'grid-delete-btn';
        btn.addEventListener('click', () => this.deleteAsset(params.data.id));
        return btn;
      }
    }
  ];

  // Grid options default
  protected readonly defaultColDef: ColDef = {
    resizable: true
  };

  ngOnInit(): void {
    this.finance.loadAssets();
  }

  onGridReady(params: GridReadyEvent): void {
    this.gridApi = params.api;
  }

  // Handle cell edits directly inside the AG Grid
  onCellValueChanged(event: CellValueChangedEvent): void {
    const updatedAsset: Asset = event.data;
    // Calculate new current value as quantity * unit_cost if cell is quantity or unit_cost
    if (event.colDef.field === 'quantity' || event.colDef.field === 'unit_cost') {
      const qty = parseFloat(updatedAsset.quantity) || 0;
      const cost = parseFloat(updatedAsset.unit_cost) || 0;
      updatedAsset.current_value = (qty * cost).toString();
    }

    this.finance.patchAsset(updatedAsset.id, updatedAsset).subscribe({
      next: () => {
        // Silently recompute insights
        this.finance.recompute().subscribe();
      },
      error: () => {
        // Rollback grid cell if save fails
        this.finance.loadAssets();
      }
    });
  }

  // Keyboard navigation & Form submission
  onAddSubmit(): void {
    if (this.assetForm.invalid) {
      this.assetForm.markAllAsTouched();
      return;
    }
    const val = this.assetForm.getRawValue();
    const qty = parseFloat(val.quantity) || 0;
    const cost = parseFloat(val.unit_cost) || 0;

    const newAsset: Partial<Asset> = {
      ...val,
      current_value: (qty * cost).toString()
    };

    this.finance.createAsset(newAsset).subscribe({
      next: () => {
        this.isAdding.set(false);
        this.assetForm.reset({
          name: '', ticker: '', isin: '', asset_class: 'cash',
          quantity: '0', unit_cost: '0', currency: 'SGD', exchange: '',
          country: '', notes: ''
        });
        this.finance.loadAssets();
        // Recompute insights
        this.finance.recompute().subscribe();
      }
    });
  }

  deleteAsset(id: string): void {
    if (confirm('Are you sure you want to delete this asset?')) {
      this.finance.deleteAsset(id).subscribe({
        next: () => {
          this.finance.loadAssets();
          this.finance.recompute().subscribe();
        }
      });
    }
  }

  // CSV Bulk Import
  onFileSelected(event: any): void {
    const file: File = event.target.files[0];
    if (!file) return;

    this.uploadError.set(null);
    this.importResult.set(null);

    this.finance.importAssets(file).subscribe({
      next: (res) => {
        this.importResult.set(res);
        this.finance.loadAssets();
        this.finance.recompute().subscribe();
      },
      error: (err) => {
        this.uploadError.set(err.error?.error || 'Failed to process CSV file.');
      }
    });
  }
}

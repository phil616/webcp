import { type App } from 'vue';
import LayoutContent from './layout-content/index.vue';
import RouterButton from './router-button/index.vue';
import ComplexTable from './table/complex/ComplexTable.vue';
import OpDialog from './del-dialog/index.vue';
import CopyButton from '@panel/components/copy-button/index.vue';
import MsgInfo from '@panel/components/msg-info/index.vue';
import DrawerPro from '@panel/components/drawer-pro/index.vue';
import DialogPro from '@panel/components/dialog-pro/index.vue';
import FuDropdownItem from '@panel/components/fu/FuDropdownItem.vue';
import FuInputRwSwitch from '@panel/components/fu/FuInputRwSwitch.vue';
import FuReadWriteSwitch from '@panel/components/fu/FuReadWriteSwitch.vue';
import FuSelectRwSwitch from '@panel/components/fu/FuSelectRwSwitch.vue';
import FuStep from '@panel/components/fu/FuStep';
import FuSteps from '@panel/components/fu/FuSteps';
import TableComponents from '@panel/components/table';
export default {
    install(app: App) {
        app.use(TableComponents);
        app.component(LayoutContent.name, LayoutContent);
        app.component(RouterButton.name, RouterButton);
        app.component(ComplexTable.name, ComplexTable);
        app.component(OpDialog.name, OpDialog);
        app.component(CopyButton.name, CopyButton);
        app.component(MsgInfo.name, MsgInfo);
        app.component(DrawerPro.name, DrawerPro);
        app.component(DialogPro.name, DialogPro);
        [FuDropdownItem, FuInputRwSwitch, FuReadWriteSwitch, FuSelectRwSwitch, FuSteps, FuStep].forEach((component) => {
            app.component((component as any).name, component as any);
        });
    },
};
